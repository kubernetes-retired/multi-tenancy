/*
Copyright 2020 The Kubernetes Authors.

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

package virtualcluster

import (
	"fmt"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
)

const (
	// vcStartTimeout is how long to wait for the vc to be started.
	vcStartTimeout = 10 * time.Minute
	// vcDeletionTimeout is how long to wait for the vc to be deleted.
	vcDeletionTimeout = 5 * time.Minute
	// poll is how often to poll vc.
	poll = 2 * time.Second
)

// WaitForVCRunningInNamespace waits default amount of time (vcStartTimeout) for the specified vc to become running.
// Returns an error if timeout occurs first, or vc goes in to failed state.
func WaitForVCRunningInNamespace(c vcclient.Interface, vcName, namespace string) error {
	return WaitTimeoutForVCRunningInNamespace(c, vcName, namespace, vcStartTimeout)
}

// WaitTimeoutForVCRunningInNamespace waits the given timeout duration for the specified vc to become running.
func WaitTimeoutForVCRunningInNamespace(c vcclient.Interface, vcName, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(poll, timeout, vcRunning(c, vcName, namespace))
}

func vcRunning(c vcclient.Interface, vcName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.TenancyV1alpha1().VirtualClusters(namespace).Get(vcName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case v1alpha1.ClusterRunning:
			return true, nil
		case v1alpha1.ClusterError:
			return false, fmt.Errorf("vc ran to error")
		}
		return false, nil
	}
}

// WaitForVCNotFoundInNamespace waits default amount of time (vcStartTimeout) for the specified vc to become deleted.
func WaitForVCNotFoundInNamespace(c vcclient.Interface, vcName, namespace string) error {
	return WaitTimeoutForVCNotFoundInNamespace(c, vcName, namespace, vcDeletionTimeout)
}

// WaitTimeoutForVCRunningInNamespace waits the given timeout duration for the specified vc to become running.
func WaitTimeoutForVCNotFoundInNamespace(c vcclient.Interface, vcName, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(poll, timeout, vcNotFound(c, vcName, namespace))
}

func vcNotFound(c vcclient.Interface, vcName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		_, err := c.TenancyV1alpha1().VirtualClusters(namespace).Get(vcName, metav1.GetOptions{})
		if apierrs.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return true, err
		}
		return false, nil
	}
}
