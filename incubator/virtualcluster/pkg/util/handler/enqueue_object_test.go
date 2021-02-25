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

package handler

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type invalidAPIObject struct {
	A string
}

type fifoQueue struct {
	queue []interface{}
}

func (q *fifoQueue) Add(item interface{}) {
	q.queue = append(q.queue, item)
}

func (q *fifoQueue) Get() (interface{}, error) {
	if len(q.queue) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	head := q.queue[0]
	q.queue = q.queue[1:]
	return head, nil
}

func TestEnqueueRequestForObject(t *testing.T) {
	clusterName := "test-cluster"
	internalQueue := &fifoQueue{}
	queue := &EnqueueRequestForObject{
		ClusterName: clusterName,
		Queue:       internalQueue,
		AttachUID:   true,
	}

	queue.OnAdd(invalidAPIObject{A: "a"})
	if item, err := internalQueue.Get(); err == nil {
		t.Errorf("expected empty queue, got %v", item)
	}

	normalObject := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "n1",
			Namespace: "ns",
			UID:       "12345",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "c1",
					Image: "image",
				},
			},
		},
	}

	expectedEnqueuedRequest := reconciler.Request{
		ClusterName: clusterName,
		NamespacedName: types.NamespacedName{
			Name:      "n1",
			Namespace: "ns",
		},
		UID: "12345",
	}

	queue.OnAdd(normalObject)
	obj, err := internalQueue.Get()
	if err != nil {
		t.Errorf("expected %v, got empty queue", expectedEnqueuedRequest)
	}
	if !equality.Semantic.DeepEqual(obj, expectedEnqueuedRequest) {
		t.Errorf("expected enqueue %v, got %v", expectedEnqueuedRequest, obj)
	}

	queue.OnUpdate(nil, normalObject)
	obj, err = internalQueue.Get()
	if err != nil {
		t.Errorf("expected %v, got empty queue", expectedEnqueuedRequest)
	}
	if !equality.Semantic.DeepEqual(obj, expectedEnqueuedRequest) {
		t.Errorf("expected enqueue %v, got %v", expectedEnqueuedRequest, obj)
	}

	queue.OnDelete(normalObject)
	obj, err = internalQueue.Get()
	if err != nil {
		t.Errorf("expected %v, got empty queue", expectedEnqueuedRequest)
	}
	if !equality.Semantic.DeepEqual(obj, expectedEnqueuedRequest) {
		t.Errorf("expected enqueue %v, got %v", expectedEnqueuedRequest, obj)
	}
}
