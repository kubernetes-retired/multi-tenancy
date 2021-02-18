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
	v1 "k8s.io/api/core/v1"
)

// SliceInfo is the input to the algorithm
type SliceInfo struct {
	Namespace string // namespace key
	Request   v1.ResourceList
	Mandatory string // if not empty, it is the cluster that the slice should go if all checks are passed
	Hint      string // if not empty, it is the preferred cluster

	Result string // scheduled cluster name
	Err    error
}

type SliceInfoArray []*SliceInfo

func (s *SliceInfoArray) Repeat(n int, namespace string, request v1.ResourceList, mandatory, hint string) {
	for i := 0; i < n; i++ {
		*s = append(*s, &SliceInfo{
			Namespace: namespace,
			Request:   request.DeepCopy(),
			Mandatory: mandatory,
			Hint:      hint,
		})
	}
}
