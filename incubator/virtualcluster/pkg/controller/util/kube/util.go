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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

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

// CreateRootNS creates the root namespace for the vc
func CreateRootNS(cli client.Client, vc *tenancyv1alpha1.Virtualcluster) (string, error) {
	nsName := conversion.ToClusterKey(vc)
	err := cli.Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Annotations: map[string]string{
				constants.LabelVCName:      vc.Name,
				constants.LabelVCNamespace: vc.Namespace,
				constants.LabelVCUID:       string(vc.UID),
				constants.LabelVCRootNS:    "true",
			},
		},
	})
	if apierrors.IsAlreadyExists(err) {
		return nsName, nil
	}
	return nsName, err
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
			log.Info("fail to get obj on patch failure", "object", vc.GetName(), "error", err.Error())
		}
		return patchErr
	})
}

// RetryUpdateVCStatusOnConflict tries to update the Virtualcluster 'vc' status. It will retry
// to update the 'vc' if there are conflicts caused by other code
func RetryUpdateVCStatusOnConflict(ctx context.Context, cli client.Client, vc *tenancyv1alpha1.Virtualcluster, log logr.Logger) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		vcStatus := vc.Status
		updateErr := cli.Update(ctx, vc)
		if updateErr != nil {
			if err := cli.Get(ctx, types.NamespacedName{
				Namespace: vc.GetNamespace(),
				Name:      vc.GetName(),
			}, vc); err != nil {
				log.Info("fail to get obj on update failure", "object", vc.GetName(), "error", err.Error())
			}
			vc.Status = vcStatus
		}
		return updateErr
	})
}

// SetVCStatus set the virtualcluster 'vc' status, and append the new status to conditions list
func SetVCStatus(vc *tenancyv1alpha1.Virtualcluster, phase tenancyv1alpha1.ClusterPhase, message, reason string) {
	vc.Status.Phase = phase
	vc.Status.Message = message
	vc.Status.Reason = reason
	vc.Status.Conditions = append(vc.Status.Conditions, tenancyv1alpha1.ClusterCondition{
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	})
}

// IsObjExist check if object with 'key' exist
func IsObjExist(cli client.Client, key client.ObjectKey, obj runtime.Object, log logr.Logger) bool {
	if err := cli.Get(context.TODO(), key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		log.Error(err, "fail to get object", "object name", key.Name, "object namespace", key.Namespace)
		return false
	}
	return true
}

// NewInClusterClient creates a client that has virtualcluster and clusterversion schemes registered
func NewInClusterClient() (client.Client, error) {
	kbCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	cliScheme := scheme.Scheme
	err = tenancyv1alpha1.AddToScheme(cliScheme)
	if err != nil {
		return nil, err
	}
	// create a new client to talk to apiserver directly
	cli, err := client.New(kbCfg, client.Options{Scheme: cliScheme})
	if err != nil {
		return nil, err
	}
	return cli, nil
}
