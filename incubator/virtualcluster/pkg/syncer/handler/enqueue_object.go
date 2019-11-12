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

package handler

import (
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type EnqueueRequestForObject struct {
	Cluster *reconciler.ClusterInfo
	Queue   Queue
}

func (e *EnqueueRequestForObject) enqueue(obj interface{}, event reconciler.EventType) {
	o, err := meta.Accessor(obj)
	if err != nil {
		return
	}

	r := reconciler.Request{Cluster: e.Cluster, Event: event, Obj: obj}
	r.Namespace = o.GetNamespace()
	r.Name = o.GetName()

	e.Queue.Add(r)
}

func (e *EnqueueRequestForObject) OnAdd(obj interface{}) {
	e.enqueue(obj, reconciler.AddEvent)
}

func (e *EnqueueRequestForObject) OnUpdate(oldObj, newObj interface{}) {
	e.enqueue(newObj, reconciler.UpdateEvent)
}

func (e *EnqueueRequestForObject) OnDelete(obj interface{}) {
	e.enqueue(obj, reconciler.DeleteEvent)
}
