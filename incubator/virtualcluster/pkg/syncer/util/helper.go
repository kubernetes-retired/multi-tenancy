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

	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

func GetVirtualClusterSpec(mc *mc.MultiClusterController, clustername string) (*v1alpha1.VirtualClusterSpec, error) {
	obj, err := mc.GetClusterObject(clustername)
	if err != nil {
		return nil, fmt.Errorf("Fail to obtain the virtualcluster object.")
	}

	vc, ok := obj.(*v1alpha1.VirtualCluster)
	if !ok {
		return nil, fmt.Errorf("Cannot get the virtualcluster spec from non-vc object.")
	}

	spec := vc.Spec.DeepCopy()
	prefixesSet := sets.NewString(spec.OpaqueMetaPrefixes...)
	if !prefixesSet.Has(constants.DefaultOpaqueMetaPrefix) {
		spec.OpaqueMetaPrefixes = append(spec.OpaqueMetaPrefixes, constants.DefaultOpaqueMetaPrefix)
	}
	prefixesSet = sets.NewString(spec.TransparentMetaPrefixes...)
	if !prefixesSet.Has(constants.DefaultTransparentMetaPrefix) {
		spec.TransparentMetaPrefixes = append(spec.TransparentMetaPrefixes, constants.DefaultTransparentMetaPrefix)
	}

	return spec, nil

}
