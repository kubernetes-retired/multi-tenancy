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

package differ

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// HandlerFuncs is an adaptor to let you easily specify as many or
// as few of the handle functions as you want while still implementing
// Handler.
type HandlerFuncs struct {
	AddFunc    func(obj ClusterObject)
	DeleteFunc func(obj ClusterObject)
	UpdateFunc func(obj1, obj2 ClusterObject)
}

// OnAdd calls AddFunc if it's not nil.
func (h HandlerFuncs) OnAdd(obj ClusterObject) {
	if h.AddFunc != nil {
		h.AddFunc(obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (h HandlerFuncs) OnUpdate(obj1, obj2 ClusterObject) {
	if h.UpdateFunc != nil {
		h.UpdateFunc(obj1, obj2)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (h HandlerFuncs) OnDelete(obj ClusterObject) {
	if h.DeleteFunc != nil {
		h.DeleteFunc(obj)
	}
}

// FilteringHandler applies the provided filter to all events coming
// in. If any object match the filter, will skip this func.
type FilteringHandler struct {
	FilterFunc func(obj ClusterObject) bool
	Handler    Handler
}

// OnAdd calls the nested handler only if the filter succeeds
func (h FilteringHandler) OnAdd(obj ClusterObject) {
	if h.FilterFunc != nil && !h.FilterFunc(obj) {
		return
	}
	h.Handler.OnAdd(obj)
}

// OnUpdate calls the nested handler only if both match the filter.
func (h FilteringHandler) OnUpdate(obj1, obj2 ClusterObject) {
	if h.FilterFunc != nil && (!h.FilterFunc(obj1) || !h.FilterFunc(obj2)) {
		return
	}
	h.Handler.OnUpdate(obj1, obj2)
}

// OnDelete calls the nested handler only if the filter succeeds
func (h FilteringHandler) OnDelete(obj ClusterObject) {
	if h.FilterFunc != nil && !h.FilterFunc(obj) {
		return
	}
	h.Handler.OnDelete(obj)
}

func DefaultDifferFilter(knownClusterSet sets.String) func(obj ClusterObject) bool {
	return func(obj ClusterObject) bool {
		// vObj
		if obj.OwnerCluster != "" {
			if knownClusterSet.Has(obj.OwnerCluster) {
				return true
			}
			return false
		}

		// pObj
		clusterName, vNamespace := conversion.GetVirtualOwner(obj)
		if clusterName != "" && vNamespace != "" && knownClusterSet.Has(clusterName) {
			return true
		}
		return false
	}
}

func DefaultClusterObjectKey(obj metav1.Object, ownerCluster string) string {
	var ns = obj.GetNamespace()
	if ownerCluster != "" {
		ns = conversion.ToSuperMasterNamespace(ownerCluster, ns)
	}
	return fmt.Sprintf("%s%c%s", ns, '/', obj.GetName())
}
