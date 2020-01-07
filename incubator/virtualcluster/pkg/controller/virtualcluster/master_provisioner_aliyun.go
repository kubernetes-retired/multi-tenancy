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

package virtualcluster

import (
	"errors"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type MasterProvisionerAliyun struct {
	client.Client
	scheme *runtime.Scheme
}

func NewMasterProvisionerAliyun(mgr manager.Manager) *MasterProvisionerAliyun {
	return &MasterProvisionerAliyun{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

func (mpa *MasterProvisionerAliyun) CreateVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	return errors.New("Not implement yet")
}

func (mpa *MasterProvisionerAliyun) DeleteVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	return errors.New("Not implement yet")
}
