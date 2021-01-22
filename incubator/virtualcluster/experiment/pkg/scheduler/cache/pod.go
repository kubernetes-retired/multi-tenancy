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

	v1 "k8s.io/api/core/v1"
)

type Pod struct {
	owner     string //tenant cluster name
	namespace string
	name      string
	uid       string

	request v1.ResourceList

	cluster string // the scheduled cluster
}

func NewPod(owner, namespace, name, uid, cluster string, request v1.ResourceList) *Pod {
	return &Pod{
		owner:     owner,
		namespace: namespace,
		name:      name,
		uid:       uid,
		request:   request,
		cluster:   cluster,
	}
}

func (p *Pod) DeepCopy() *Pod {
	return NewPod(p.owner, p.namespace, p.name, p.uid, p.cluster, p.request.DeepCopy())
}

func (p *Pod) GetNamespaceKey() string {
	return fmt.Sprintf("%s/%s", p.owner, p.namespace)
}

func (p *Pod) GetKey() string {
	return fmt.Sprintf("%s/%s", p.GetNamespaceKey(), p.uid)
}
