/*
Copyright 2021 The Kubernetes Authors.

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

package util

import (
	"fmt"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

func GetVirtualClusterObject(mc *mc.MultiClusterController, clustername string) (*v1alpha1.VirtualCluster, error) {
	obj, err := mc.GetClusterObject(clustername)
	if err != nil {
		return nil, fmt.Errorf("fail to obtain the virtualcluster object")
	}

	vc, ok := obj.(*v1alpha1.VirtualCluster)
	if !ok {
		return nil, fmt.Errorf("cannot get the virtualcluster from non-vc object")
	}

	return vc, nil
}
