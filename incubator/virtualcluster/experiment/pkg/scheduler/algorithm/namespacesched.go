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

package algorithm

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
)

func ScheduleNamespaceSlices(slices SliceInfoArray, snapshot *internalcache.NamespaceSchedSnapshot) SliceInfoArray {
	for i, each := range slices {
		ret, err := ScheduleOneSlice(each, snapshot)
		if err != nil {
			slices[i].Err = err
		} else {
			slices[i].Result = ret
			snapshot.AddSlices([]*internalcache.Slice{internalcache.NewSlice(each.Namespace, each.Request, ret)})
		}
	}
	return slices
}

func ScheduleOneSlice(slice *SliceInfo, snapshot *internalcache.NamespaceSchedSnapshot) (string, error) {
	var err error
	if slice.Mandatory != "" {
		cluster, exists := snapshot.GetClusterUsageMap()[slice.Mandatory]
		if !exists {
			return "", fmt.Errorf("mandatory cluster %s cannot be found", slice.Mandatory)
		}

		if err = fitSlice(slice.Request, cluster); err != nil {
			return "", fmt.Errorf("mandatory request cannot be satisfied %v ", err)
		}
		return slice.Mandatory, nil
	}

	if slice.Hint != "" {
		cluster, exists := snapshot.GetClusterUsageMap()[slice.Hint]
		if !exists {
			if err = fitSlice(slice.Request, cluster); err == nil {
				return slice.Hint, nil
			}
		}
	}

	// First fit
	for n, cluster := range snapshot.GetClusterUsageMap() {
		if err = fitSlice(slice.Request, cluster); err == nil {
			return n, nil
		}
	}
	// return the last error
	return "", err
}

func fitSlice(request v1.ResourceList, cluster *internalcache.ClusterUsage) error {
	used := cluster.GetMaxAlloc()

	for res, avail := range cluster.GetCapacity() {
		allocAfter := used[res]
		allocAfter.Add(request[res])
		if avail.Cmp(allocAfter) < 0 {
			return fmt.Errorf("resource %v cannot be fit, avail %v, request %v, allocAfter %v", res, avail, request[res], allocAfter)
		}
	}
	return nil
}

func SchedulePod(pod *internalcache.Pod, snapshot *internalcache.PodSchedSnapshot) (string, error) {
	var err error
	// First fit
	for name, cluster := range snapshot.GetClusterUsageMap() {
		if err := fitSlice(pod.GetRequest(), cluster); err == nil {
			return name, nil
		}
	}
	// return the last error
	return "", err
}
