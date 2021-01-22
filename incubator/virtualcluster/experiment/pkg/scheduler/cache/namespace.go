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

package cache

import (
	"fmt"
	"math"

	v1 "k8s.io/api/core/v1"
)

type Placement struct {
	cluster string
	num     int
}

func NewPlacement(cluster string, num int) *Placement {
	return &Placement{
		cluster: cluster,
		num:     num,
	}
}

type Namespace struct {
	owner  string //tenant cluster name
	name   string
	labels map[string]string

	quota      v1.ResourceList
	quotaSlice v1.ResourceList

	schedule []*Placement
}

type Slice struct {
	owner   string // namespace key
	size    v1.ResourceList
	cluster string
}

func NewSlice(owner string, size v1.ResourceList, cluster string) *Slice {
	return &Slice{
		owner:   owner,
		size:    size,
		cluster: cluster,
	}
}

func (s Slice) DeepCopy() *Slice {
	return NewSlice(s.owner, s.size.DeepCopy(), s.cluster)
}

func NewNamespace(owner, name string, labels map[string]string, quota, quotaSlice v1.ResourceList, schedule []*Placement) *Namespace {
	return &Namespace{
		owner:      owner,
		name:       name,
		labels:     labels,
		quota:      quota,
		quotaSlice: quotaSlice,
		schedule:   schedule,
	}
}

func (n *Namespace) DeepCopy() *Namespace {
	schedCopy := make([]*Placement, 0, len(n.schedule))
	for _, each := range n.schedule {
		schedCopy = append(schedCopy, NewPlacement(each.cluster, each.num))
	}
	labelCopy := make(map[string]string)
	for k, v := range n.labels {
		labelCopy[k] = v
	}
	return NewNamespace(n.owner, n.name, labelCopy, n.quota.DeepCopy(), n.quotaSlice.DeepCopy(), schedCopy)

}

func (n *Namespace) GetKey() string {
	return fmt.Sprintf("%s/%s", n.owner, n.name)
}

func GetNumSlices(quota, quotaSlice v1.ResourceList) (int, error) {
	more := make(map[v1.ResourceName]struct{})
	for k, _ := range quota {
		more[k] = struct{}{}
	}
	num := 0
	for k, v := range quotaSlice {
		q, ok := quota[k]

		if !ok {
			return 0, fmt.Errorf("quota slice resouce %v is missing from quota", k)
		}
		if v.Value() == 0 {
			return 0, fmt.Errorf("quota slice resource %v has value of 0", k)
		}
		delete(more, k)
		if q.Cmp(v) == -1 {
			return 0, fmt.Errorf("quota slice is larger than quota for resource %v", k)
		}
		n := math.Ceil(float64(q.Value()) / float64(v.Value()))
		if n > float64(num) {
			num = int(n)
		}
	}
	if len(more) != 0 {
		return 0, fmt.Errorf("quota slice has missing resouces %v", more)
	}
	return num, nil
}
