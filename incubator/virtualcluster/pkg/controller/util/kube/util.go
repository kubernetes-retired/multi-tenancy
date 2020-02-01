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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			return fmt.Errorf("%s/%s is not ready in %s seconds", timeOutSec)
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

// CreateNS create namespace 'nsName' by client 'cli'
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
