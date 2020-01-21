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
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type VCMutateInterface interface {
	Pod(pPod *v1.Pod) PodMutateInterface
	Service(pService *v1.Service) ServiceMutateInterface
}

type mutator struct {
	vc mccontroller.ClusterInterface
}

func VC(vc mccontroller.ClusterInterface) VCMutateInterface {
	return &mutator{vc: vc}
}

func (m *mutator) Pod(pPod *v1.Pod) PodMutateInterface {
	return &podMutator{vc: m.vc, pPod: pPod}
}

func (m *mutator) Service(pService *v1.Service) ServiceMutateInterface {
	return &serviceMutator{vc: m.vc, pService: pService}
}

type PodMutateInterface interface {
	Mutate(pPod *v1.Pod, vSASecret, SASecret *v1.Secret, services []*v1.Service, nameServer string) error
}

type podMutator struct {
	vc   mccontroller.ClusterInterface
	pPod *v1.Pod
}

// MutatePod convert the meta data of containers to super master namespace.
// replace the service account token volume mounts to super master side one.
func (p *podMutator) Mutate(vPod *v1.Pod, vSASecret, SASecret *v1.Secret, services []*v1.Service, nameServer string) error {
	p.pPod.Status = v1.PodStatus{}
	p.pPod.Spec.NodeName = ""

	// setup env var map
	serviceEnv := getServiceEnvVarMap(p.pPod.Namespace, *p.pPod.Spec.EnableServiceLinks, services)

	for i := range p.pPod.Spec.Containers {
		mutateContainerEnv(&p.pPod.Spec.Containers[i], vPod, serviceEnv)
		mutateContainerSecret(&p.pPod.Spec.Containers[i], vSASecret, SASecret)
	}

	for i := range p.pPod.Spec.InitContainers {
		mutateContainerEnv(&p.pPod.Spec.InitContainers[i], vPod, serviceEnv)
		mutateContainerSecret(&p.pPod.Spec.InitContainers[i], vSASecret, SASecret)
	}

	for i, volume := range p.pPod.Spec.Volumes {
		if volume.Name == vSASecret.Name {
			p.pPod.Spec.Volumes[i].Name = SASecret.Name
			p.pPod.Spec.Volumes[i].Secret.SecretName = SASecret.Name
		}
	}

	p.mutateDNSConfig(vPod, nameServer)

	// FIXME(zhuangqh): how to support pod subdomain.
	if p.pPod.Spec.Subdomain != "" {
		p.pPod.Spec.Subdomain = ""
	}

	return p.attachTenantMeta(vPod)
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

func mutateContainerSecret(c *v1.Container, vSASecret, SASecret *v1.Secret) {
	for j, volumeMount := range c.VolumeMounts {
		if volumeMount.Name == vSASecret.Name {
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

func getServiceEnvVarMap(ns string, enableServiceLinks bool, services []*v1.Service) map[string]string {
	var (
		serviceMap = make(map[string]*v1.Service)
		m          = make(map[string]string)
	)

	// project the services in namespace ns onto the master services
	for i := range services {
		service := services[i]
		// ignore services where ClusterIP is "None" or empty
		if !v1helper.IsServiceIPSet(service) {
			continue
		}
		serviceName := service.Name

		// We always want to add environment variabled for master services
		// from the master service namespace, even if enableServiceLinks is false.
		// We also add environment variables for other services in the same
		// namespace, if enableServiceLinks is true.
		if service.Namespace == masterServiceNamespace && masterServices.Has(serviceName) {
			if _, exists := serviceMap[serviceName]; !exists {
				serviceMap[serviceName] = service
			}
		} else if service.Namespace == ns && enableServiceLinks {
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
	return m
}

func (p *podMutator) mutateDNSConfig(vPod *v1.Pod, nameServer string) {
	dnsPolicy := p.pPod.Spec.DNSPolicy

	switch dnsPolicy {
	case v1.DNSNone:
		return
	case v1.DNSClusterFirstWithHostNet:
		p.mutateClusterFirstDNS(vPod, nameServer)
		return
	case v1.DNSClusterFirst:
		if !p.pPod.Spec.HostNetwork {
			p.mutateClusterFirstDNS(vPod, nameServer)
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

func (p *podMutator) mutateClusterFirstDNS(vPod *v1.Pod, nameServer string) {
	if nameServer == "" {
		klog.Infof("vc %s does not have ClusterDNS IP configured and cannot create Pod using %q policy. Falling back to %q policy.",
			p.vc.GetClusterName(), v1.DNSClusterFirst, v1.DNSDefault)
		p.pPod.Spec.DNSPolicy = v1.DNSDefault
		return
	}

	existingDNSConfig := p.pPod.Spec.DNSConfig

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

	if clusterDomain := p.vc.GetSpec().ClusterDomain; clusterDomain != "" {
		nsSvcDomain := fmt.Sprintf("%s.svc.%s", vPod.Namespace, clusterDomain)
		svcDomain := fmt.Sprintf("svc.%s", clusterDomain)
		dnsConfig.Searches = []string{nsSvcDomain, svcDomain, clusterDomain}
	}

	if existingDNSConfig != nil {
		dnsConfig.Nameservers = omitDuplicates(append(existingDNSConfig.Nameservers, dnsConfig.Nameservers...))
		dnsConfig.Searches = omitDuplicates(append(existingDNSConfig.Searches, dnsConfig.Searches...))
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

// TODO(zhuangqh): make this plugable.
// for now, only Deployment Pods are mutated.
func (p *podMutator) attachTenantMeta(vPod *v1.Pod) error {
	if len(vPod.ObjectMeta.OwnerReferences) == 0 {
		return nil
	}

	if vPod.ObjectMeta.OwnerReferences[0].Kind != "ReplicaSet" {
		return nil
	}

	ns := vPod.ObjectMeta.Namespace
	replicaSetName := vPod.ObjectMeta.OwnerReferences[0].Name
	client, err := p.vc.GetClient()
	if err != nil {
		return fmt.Errorf("vc %s failed to get client: %v", p.vc.GetClusterName(), err)
	}
	replicaSetObj, err := client.AppsV1().ReplicaSets(ns).Get(replicaSetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("vc %s failed to get replicaset object %s in %s: %v", p.vc.GetClusterName(), replicaSetName, ns, err)
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

type ServiceMutateInterface interface {
	Mutate(vService *v1.Service)
}

type serviceMutator struct {
	vc       mccontroller.ClusterInterface
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
