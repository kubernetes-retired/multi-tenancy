/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conversion

import (
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/kubelet/envvars"
	"k8s.io/utils/pointer"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type VCMutateInterface interface {
	Pod(pPod *v1.Pod) PodMutateInterface
	Service(pService *v1.Service) ServiceMutateInterface
	ServiceAccountTokenSecret(pSecret *v1.Secret) SecretMutateInterface
}

type mutator struct {
	mc          *mc.MultiClusterController
	clusterName string
}

func VC(mc *mc.MultiClusterController, clusterName string) VCMutateInterface {
	return &mutator{mc: mc, clusterName: clusterName}
}

func (m *mutator) Pod(pPod *v1.Pod) PodMutateInterface {
	return &podMutateCtx{mc: m.mc, clusterName: m.clusterName, pPod: pPod}
}

func (m *mutator) Service(pService *v1.Service) ServiceMutateInterface {
	return &serviceMutator{pService: pService}
}

func (m *mutator) ServiceAccountTokenSecret(pSecret *v1.Secret) SecretMutateInterface {
	return &saSecretMutator{pSecret: pSecret}
}

type PodMutateInterface interface {
	Mutate(ms ...PodMutator) error
}

type PodMutator func(p *podMutateCtx) error

type podMutateCtx struct {
	mc          *mc.MultiClusterController
	clusterName string
	pPod        *v1.Pod
}

// MutatePod convert the meta data of containers to super master namespace.
// replace the service account token volume mounts to super master side one.
func (p *podMutateCtx) Mutate(ms ...PodMutator) error {
	for _, mutator := range ms {
		if err := mutator(p); err != nil {
			return err
		}
	}

	return nil
}

func mutatePodAffinityTerms(terms []v1.PodAffinityTerm, clusterName string) {
	for i, each := range terms {
		if each.LabelSelector != nil {
			if terms[i].LabelSelector.MatchLabels == nil {
				terms[i].LabelSelector.MatchLabels = make(map[string]string)
			}
			terms[i].LabelSelector.MatchLabels[constants.LabelCluster] = clusterName
		}
	}
}

func mutateWeightedPodAffinityTerms(weightedTerms []v1.WeightedPodAffinityTerm, clusterName string) {
	for i, each := range weightedTerms {
		if each.PodAffinityTerm.LabelSelector != nil {
			if weightedTerms[i].PodAffinityTerm.LabelSelector.MatchLabels == nil {
				weightedTerms[i].PodAffinityTerm.LabelSelector.MatchLabels = make(map[string]string)
			}
			weightedTerms[i].PodAffinityTerm.LabelSelector.MatchLabels[constants.LabelCluster] = clusterName
		}
	}
}

func PodMutateDefault(vPod *v1.Pod, SASecret *v1.Secret, services []*v1.Service, nameServer string) PodMutator {
	return func(p *podMutateCtx) error {
		p.pPod.Status = v1.PodStatus{}
		p.pPod.Spec.NodeName = ""

		// setup env var map
		apiServerClusterIP, serviceEnv := getServiceEnvVarMap(p.pPod.Namespace, p.clusterName, p.pPod.Spec.EnableServiceLinks, services)

		// if apiServerClusterIP is empty, just let it fails.
		p.pPod.Spec.HostAliases = append(p.pPod.Spec.HostAliases, v1.HostAlias{
			IP:        apiServerClusterIP,
			Hostnames: []string{"kubernetes", "kubernetes.default", "kubernetes.default.svc"},
		})

		for i := range p.pPod.Spec.Containers {
			mutateContainerEnv(&p.pPod.Spec.Containers[i], vPod, serviceEnv)
			mutateContainerSecret(&p.pPod.Spec.Containers[i], SASecret)
		}

		for i := range p.pPod.Spec.InitContainers {
			mutateContainerEnv(&p.pPod.Spec.InitContainers[i], vPod, serviceEnv)
			mutateContainerSecret(&p.pPod.Spec.InitContainers[i], SASecret)
		}

		for i, volume := range p.pPod.Spec.Volumes {
			if volume.Name == SASecret.Annotations[constants.LabelSecretName] {
				p.pPod.Spec.Volumes[i].Name = SASecret.Name
				p.pPod.Spec.Volumes[i].Secret.SecretName = SASecret.Name
			}
		}

		// Make sure pod-pod affinity/anti-affinity rules are applied within tenant scope.
		// First, add a label to mark Pod's tenant. This is indeed a dup of the same key:val in annotation.
		// TODO: consider removing the dup key:val in annotation.
		labels := p.pPod.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.LabelCluster] = p.clusterName
		p.pPod.SetLabels(labels)
		// Then add tenant label to all affinity terms if any.
		if p.pPod.Spec.Affinity != nil && p.pPod.Spec.Affinity.PodAffinity != nil {
			mutatePodAffinityTerms(p.pPod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution, p.clusterName)
			mutateWeightedPodAffinityTerms(p.pPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution, p.clusterName)
		}

		if p.pPod.Spec.Affinity != nil && p.pPod.Spec.Affinity.PodAntiAffinity != nil {
			mutatePodAffinityTerms(p.pPod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, p.clusterName)
			mutateWeightedPodAffinityTerms(p.pPod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution, p.clusterName)
		}

		clusterDomain, err := p.mc.GetClusterDomain(p.clusterName)
		if err != nil {
			return err
		}
		mutateDNSConfig(p, vPod, clusterDomain, nameServer)

		// FIXME(zhuangqh): how to support pod subdomain.
		if p.pPod.Spec.Subdomain != "" {
			p.pPod.Spec.Subdomain = ""
		}

		return nil
	}
}

func mutateContainerEnv(c *v1.Container, vPod *v1.Pod, serviceEnvMap map[string]string) {
	// Inject env var from service
	// 1. Do nothing if it conflicts with user-defined one.
	// 2. Add remaining service environment vars
	envNameMap := make(map[string]struct{})
	for j, env := range c.Env {
		mutateDownwardAPIField(&c.Env[j], vPod)
		envNameMap[env.Name] = struct{}{}
	}
	for k, v := range serviceEnvMap {
		if _, exists := envNameMap[k]; !exists {
			c.Env = append(c.Env, v1.EnvVar{Name: k, Value: v})
		}
	}
}

func mutateContainerSecret(c *v1.Container, SASecret *v1.Secret) {
	for j, volumeMount := range c.VolumeMounts {
		if volumeMount.Name == SASecret.Annotations[constants.LabelSecretName] {
			c.VolumeMounts[j].Name = SASecret.Name
		}
	}
}

func mutateDownwardAPIField(env *v1.EnvVar, vPod *v1.Pod) {
	if env.ValueFrom == nil {
		return
	}
	if env.ValueFrom.FieldRef == nil {
		return
	}
	if !strings.HasPrefix(env.ValueFrom.FieldRef.FieldPath, "metadata") {
		return
	}
	switch env.ValueFrom.FieldRef.FieldPath {
	case "metadata.name":
		env.Value = vPod.Name
	case "metadata.namespace":
		env.Value = vPod.Namespace
	case "metadata.uid":
		env.Value = string(vPod.UID)
	}
	env.ValueFrom = nil
}

func getServiceEnvVarMap(ns, cluster string, enableServiceLinks *bool, services []*v1.Service) (string, map[string]string) {
	var (
		serviceMap       = make(map[string]*v1.Service)
		m                = make(map[string]string)
		apiServerService string
	)

	// the master service namespace of the given virtualcluster
	tenantMasterSvcNs := ToSuperMasterNamespace(cluster, masterServiceNamespace)

	// project the services in namespace ns onto the master services
	for i := range services {
		service := services[i]
		// ignore services where ClusterIP is "None" or empty
		if !v1helper.IsServiceIPSet(service) {
			continue
		}
		serviceName := service.Name

		// We always want to add environment variabled for master services
		// from the corresponding master service namespace of the virtualcluster,
		// even if enableServiceLinks is false.
		// We also add environment variables for other services in the same
		// namespace, if enableServiceLinks is true.
		if service.Namespace == tenantMasterSvcNs && masterServices.Has(serviceName) {
			apiServerService = service.Spec.ClusterIP
			if _, exists := serviceMap[serviceName]; !exists {
				serviceMap[serviceName] = service
			}
		} else if service.Namespace == ns && enableServiceLinks != nil && *enableServiceLinks {
			serviceMap[serviceName] = service
		}
	}

	var mappedServices []*v1.Service
	for key := range serviceMap {
		mappedServices = append(mappedServices, serviceMap[key])
	}

	for _, e := range envvars.FromServices(mappedServices) {
		m[e.Name] = e.Value
	}
	return apiServerService, m
}

func mutateDNSConfig(p *podMutateCtx, vPod *v1.Pod, clusterDomain, nameServer string) {
	dnsPolicy := p.pPod.Spec.DNSPolicy

	switch dnsPolicy {
	case v1.DNSNone:
		return
	case v1.DNSClusterFirstWithHostNet:
		mutateClusterFirstDNS(p, vPod, clusterDomain, nameServer)
		return
	case v1.DNSClusterFirst:
		if !p.pPod.Spec.HostNetwork {
			mutateClusterFirstDNS(p, vPod, clusterDomain, nameServer)
			return
		}
		// Fallback to DNSDefault for pod on hostnetwork.
		fallthrough
	case v1.DNSDefault:
		// FIXME(zhuangqh): allow host dns or not.
		p.pPod.Spec.DNSPolicy = v1.DNSNone
		return
	}
}

func mutateClusterFirstDNS(p *podMutateCtx, vPod *v1.Pod, clusterDomain, nameServer string) {
	if nameServer == "" {
		klog.Infof("vc %s does not have ClusterDNS IP configured and cannot create Pod using %q policy. Falling back to %q policy.",
			p.clusterName, v1.DNSClusterFirst, v1.DNSDefault)
		p.pPod.Spec.DNSPolicy = v1.DNSDefault
		return
	}

	// For a pod with DNSClusterFirst policy, the cluster DNS server is
	// the only nameserver configured for the pod. The cluster DNS server
	// itself will forward queries to other nameservers that is configured
	// to use, in case the cluster DNS server cannot resolve the DNS query
	// itself.
	// FIXME(zhuangqh): tenant configure more dns server.
	dnsConfig := &v1.PodDNSConfig{
		Nameservers: []string{nameServer},
		Options: []v1.PodDNSConfigOption{
			{
				Name:  "ndots",
				Value: pointer.StringPtr("5"),
			},
		},
	}

	if clusterDomain != "" {
		nsSvcDomain := fmt.Sprintf("%s.svc.%s", vPod.Namespace, clusterDomain)
		svcDomain := fmt.Sprintf("svc.%s", clusterDomain)
		dnsConfig.Searches = []string{nsSvcDomain, svcDomain, clusterDomain}
	}

	existingDNSConfig := p.pPod.Spec.DNSConfig
	if existingDNSConfig != nil {
		dnsConfig.Nameservers = omitDuplicates(append(dnsConfig.Nameservers, existingDNSConfig.Nameservers...))
		dnsConfig.Searches = omitDuplicates(append(dnsConfig.Searches, existingDNSConfig.Searches...))
	}

	p.pPod.Spec.DNSPolicy = v1.DNSNone
	p.pPod.Spec.DNSConfig = dnsConfig
}

func omitDuplicates(strs []string) []string {
	uniqueStrs := make(map[string]bool)

	var ret []string
	for _, str := range strs {
		if !uniqueStrs[str] {
			ret = append(ret, str)
			uniqueStrs[str] = true
		}
	}
	return ret
}

// for now, only Deployment Pods are mutated.
func PodAddExtensionMeta(vPod *v1.Pod) PodMutator {
	return func(p *podMutateCtx) error {
		if len(vPod.ObjectMeta.OwnerReferences) == 0 || vPod.ObjectMeta.OwnerReferences[0].Kind != "ReplicaSet" {
			return nil
		}

		ns := vPod.ObjectMeta.Namespace
		replicaSetName := vPod.ObjectMeta.OwnerReferences[0].Name
		client, err := p.mc.GetClusterClient(p.clusterName)
		if err != nil {
			return fmt.Errorf("vc %s failed to get client: %v", p.clusterName, err)
		}
		replicaSetObj, err := client.AppsV1().ReplicaSets(ns).Get(replicaSetName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("vc %s failed to get replicaset object %s in %s: %v", p.clusterName, replicaSetName, ns, err)
		}

		if len(replicaSetObj.ObjectMeta.OwnerReferences) == 0 {
			// It can be a standalone rs
			return nil
		}
		labels := p.pPod.GetLabels()
		if len(labels) == 0 {
			labels = make(map[string]string)
		}
		labels[constants.LabelExtendDeploymentName] = replicaSetObj.ObjectMeta.OwnerReferences[0].Name
		labels[constants.LabelExtendDeploymentUID] = string(replicaSetObj.ObjectMeta.OwnerReferences[0].UID)
		p.pPod.SetLabels(labels)

		return nil
	}
}

func PodMutateAutoMountServiceAccountToken(disable bool) PodMutator {
	return func(p *podMutateCtx) error {
		if disable {
			p.pPod.Spec.AutomountServiceAccountToken = pointer.BoolPtr(false)
		}
		return nil
	}
}

type ServiceMutateInterface interface {
	Mutate(vService *v1.Service)
}

type serviceMutator struct {
	pService *v1.Service
}

func (s *serviceMutator) Mutate(vService *v1.Service) {
	if v1helper.IsServiceIPSet(vService) {
		anno := s.pService.GetAnnotations()
		if len(anno) == 0 {
			anno = make(map[string]string)
		}
		anno[constants.LabelClusterIP] = vService.Spec.ClusterIP
		s.pService.SetAnnotations(anno)
		s.pService.Spec.ClusterIP = ""
	}
	for i := range s.pService.Spec.Ports {
		s.pService.Spec.Ports[i].NodePort = 0
	}
}

type SecretMutateInterface interface {
	Mutate(vSecret *v1.Secret, clusterName string)
}

type saSecretMutator struct {
	pSecret *v1.Secret
}

func (s *saSecretMutator) Mutate(vSecret *v1.Secret, clusterName string) {
	s.pSecret.Type = v1.SecretTypeOpaque
	labels := s.pSecret.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	annotations := s.pSecret.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[constants.LabelSecretName] = vSecret.Name
	labels[constants.LabelSecretUID] = string(vSecret.UID)
	labels[constants.LabelServiceAccountUID] = vSecret.GetAnnotations()[v1.ServiceAccountUIDKey]
	s.pSecret.SetLabels(labels)
	s.pSecret.SetAnnotations(annotations)

	s.pSecret.Name = ""
	s.pSecret.GenerateName = vSecret.GetAnnotations()[v1.ServiceAccountNameKey] + "-token-"
}
