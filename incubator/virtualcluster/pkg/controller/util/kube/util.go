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

package kube

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// the namespace of the pod can be found in this file
const svcAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// GetPodNsFromInside gets the namespace of the pod from inside the pod
func GetPodNsFromInside() (string, error) {
	fileContentByt, err := ioutil.ReadFile(svcAccountPath)
	if err != nil {
		return "", err
	}
	if len(fileContentByt) == 0 {
		return "", fmt.Errorf("can't get namespace from file %s", svcAccountPath)
	}
	return string(fileContentByt), nil
}

// GetSvcClusterIP gets the ClusterIP of the service 'namespace/name'
func GetSvcClusterIP(cli client.Client, namespace, name string) (string, error) {
	svc := &v1.Service{}
	if err := cli.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, svc); err != nil {
		return "", err
	}
	if svc.Spec.ClusterIP == "" {
		return "", fmt.Errorf("the clusterIP of service %s is not set", namespace+"/"+name)
	}
	return svc.Spec.ClusterIP, nil
}

// WaitStatefulSetReady checks if the statefulset 'namespace/name' can be ready within
// the 'timeout'
func WaitStatefulSetReady(cli client.Client, namespace, name string, timeOutSec, periodSec int64) error {
	timeOut := time.After(time.Duration(timeOutSec) * time.Second)
	for {
		period := time.After(time.Duration(periodSec) * time.Second)
		select {
		case <-timeOut:
			return fmt.Errorf("%s/%s is not ready in %d seconds", namespace, name, timeOutSec)
		case <-period:
			sts := &appsv1.StatefulSet{}
			if err := cli.Get(context.TODO(), types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			}, sts); err != nil {
				return err
			}
			if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
				return nil
			}
		}
	}
}

// CreateVCNS creates namespace 'nsName' by client 'cli' and add annotation
// related to 'cluster'
func CreateVCNS(cli client.Client, cluster, nsName string) error {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	superNs, err := conversion.BuildSuperMasterNamespace(cluster, ns)
	if err != nil {
		return err
	}
	err = cli.Create(context.TODO(), superNs)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// CreateNS creates namespace 'nsName' by client 'cli'
func CreateNS(cli client.Client, nsName string) error {
	err := cli.Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// RemoveNS removes namespace 'nsName' by client 'cli'
func RemoveNS(cli client.Client, nsName string) error {
	if err := cli.Delete(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// AnnotateVC add the annotation('key'='val') to the Virtualcluster 'vc'
func AnnotateVC(cli client.Client, vc *tenancyv1alpha1.Virtualcluster, key, val string, log logr.Logger) error {
	annPatch := client.ConstantPatch(types.MergePatchType,
		[]byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, val)))
	if err := RetryPatchVCOnConflict(context.TODO(), cli, vc, annPatch, log); err != nil {
		return err
	}
	log.Info("add annotation to vc", "vc", vc.GetName(), "key", key, "value", val)
	return nil
}

// RetryPatchVCOnConflict tries to patch the Virtualcluster 'vc'. It will retry
// to patch the 'vc' if there are conflicts caused by other code
func RetryPatchVCOnConflict(ctx context.Context, cli client.Client, vc *tenancyv1alpha1.Virtualcluster, patch client.Patch, log logr.Logger, opts ...client.PatchOption) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		patchErr := cli.Patch(ctx, vc, patch, opts...)
		if err := cli.Get(ctx, types.NamespacedName{
			Namespace: vc.GetNamespace(),
			Name:      vc.GetName(),
		}, vc); err != nil {
			log.Info("fail to get obj on patch failure", "object", "error", err.Error())
		}
		return patchErr
	})
}

// isAffiliated checks if the given namespace is related to vc, i.e. contains
// annotation "tenancy.x-k8s.io/cluster":conversion.ToClusterKey(vc)
// NOTE the root ns doesn't contain this annotation
func isAffiliated(ns *v1.Namespace, vc *tenancyv1alpha1.Virtualcluster) bool {
	var tmpVcName string
	for k, v := range ns.GetAnnotations() {
		if k == constants.LabelCluster {
			tmpVcName = v
			break
		}
	}
	if tmpVcName != "" && tmpVcName == conversion.ToClusterKey(vc) {
		return true
	}
	return false

}

// DeleteAffiliatedNs deletes namespaces affiliated to the target virtualcluster
func DeleteAffiliatedNs(cli client.Client, vc *tenancyv1alpha1.Virtualcluster, log logr.Logger) error {
	// delete all related ns, except the root ns
	nsLst := &v1.NamespaceList{}
	if err := cli.List(context.TODO(), nsLst); err != nil {
		log.Error(err, "fail to list all namespaces")
		return err
	}
	for _, ns := range nsLst.Items {
		if isAffiliated(&ns, vc) {
			// remove related ns
			if err := RemoveNS(cli, ns.GetName()); err != nil {
				log.Error(err, "fail to delete the namespace", "ns", ns.GetName())
				return err
			}
			log.Info("namespace is deleted", "ns", ns.GetName())
		}
	}
	// delete the root ns
	if err := RemoveNS(cli, conversion.ToClusterKey(vc)); err != nil {
		log.Error(err, "fail to delete the root namespace", "ns", conversion.ToClusterKey(vc))
		return err
	}
	return nil
}
