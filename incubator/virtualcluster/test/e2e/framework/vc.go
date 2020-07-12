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

package framework

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned/typed/tenancy/v1alpha1"
)

// VCClient is a struct for vc client.
type VCClient struct {
	f         *Framework
	Interface clientset.Interface
	tenancyv1alpha1.VirtualClusterInterface
}

// VCClient is a convenience method for getting a vc client interface in the framework's namespace,
// possibly applying test-suite specific transformations to the vc spec.
func (f *Framework) VCClient() *VCClient {
	return &VCClient{
		f:                       f,
		Interface:               f.ClientSet,
		VirtualClusterInterface: f.VCClientSet.TenancyV1alpha1().VirtualClusters(f.Namespace.Name),
	}
}

// CreateSync creates a new vc according to the framework specifications, and wait for it to start.
func (c *VCClient) CreateSync(vc *v1alpha1.VirtualCluster) *v1alpha1.VirtualCluster {
	v, err := c.Create(vc)
	ExpectNoError(err, "failed to create vc")
	ExpectNoError(c.f.WaitForVCRunning(vc.Name))
	v, err = c.Get(v.Name, metav1.GetOptions{})
	ExpectNoError(err)
	return v
}

// DeleteSync deletes the vc and wait for the vc to disappear.
func (c *VCClient) DeleteSync(name string, options *metav1.DeleteOptions) {
	err := c.Delete(name, options)
	ExpectNoError(err, "failed to delete vc")
	ExpectNoError(c.f.WaitForVCNotFound(name), "waiting virtualcluster to be completed deleted")
}
