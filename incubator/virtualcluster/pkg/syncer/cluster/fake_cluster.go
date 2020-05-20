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

package cluster

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientgocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type fakeCluster struct {
	key           string
	vc            *v1alpha1.VirtualCluster
	fakeClientset clientset.Interface
	fakeClient    client.Client
}

var _ mccontroller.ClusterInterface = &fakeCluster{}

func NewFakeTenantCluster(vc *v1alpha1.VirtualCluster, fakeClientSet clientset.Interface, fakeClient client.Client) (*fakeCluster, error) {
	cluster := &fakeCluster{
		key:           conversion.ToClusterKey(vc),
		vc:            vc,
		fakeClientset: fakeClientSet,
		fakeClient:    fakeClient,
	}

	return cluster, nil
}

func (c *fakeCluster) GetClusterName() string {
	return c.key
}

func (c *fakeCluster) GetOwnerInfo() (string, string, string) {
	return c.vc.Name, c.vc.Namespace, string(c.vc.UID)
}

func (c *fakeCluster) GetSpec() (*v1alpha1.VirtualClusterSpec, error) {
	spec := c.vc.Spec.DeepCopy()
	prefixesSet := sets.NewString(spec.OpaqueMetaPrefixes...)
	if !prefixesSet.Has(constants.DefaultOpaqueMetaPrefix) {
		spec.OpaqueMetaPrefixes = append(spec.OpaqueMetaPrefixes, constants.DefaultOpaqueMetaPrefix)
	}
	return spec, nil
}

func (c *fakeCluster) GetClientSet() (clientset.Interface, error) {
	return c.fakeClientset, nil
}

func (c *fakeCluster) GetDelegatingClient() (client.Client, error) {
	return c.fakeClient, nil
}

func (c *fakeCluster) AddEventHandler(runtime.Object, clientgocache.ResourceEventHandler) error {
	// do nothing. we manually enqueue event in test.
	return nil
}

func (c *fakeCluster) Start() error {
	return nil
}

func (c *fakeCluster) WaitForCacheSync() bool {
	return true
}

func (c *fakeCluster) Stop() {
	return
}
