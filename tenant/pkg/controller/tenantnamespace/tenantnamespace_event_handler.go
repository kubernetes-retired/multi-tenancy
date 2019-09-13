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

package tenantnamespace

import (
	tenancyv1alpha1 "github.com/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var ownerType = &tenancyv1alpha1.TenantNamespace{}

type enqueueTenantNamespace struct {
	// groupKind is the cached Group and Kind from OwnerType
	groupKind schema.GroupKind
}

// Don't watch namespace creation for now
func (e *enqueueTenantNamespace) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueTenantNamespace) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.Add(req)
	}
}

// Don't watch unknown namespace event for now
func (e *enqueueTenantNamespace) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

// Don't watch namespace update for now
func (e *enqueueTenantNamespace) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueTenantNamespace) getOwnerReconcileRequest(object metav1.Object) []reconcile.Request {
	var result []reconcile.Request
	for _, ref := range object.GetOwnerReferences() {
		// Parse the Group out of the OwnerReference to compare it to what was parsed out of the requested OwnerType
		refGV, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			log.Error(err, "Could not parse OwnerReference APIVersion",
				"api version", ref.APIVersion)
			return nil
		}
		if ref.Kind == e.groupKind.Kind && refGV.Group == e.groupKind.Group {
			annotations := object.GetAnnotations()
			if annotations == nil || annotations[TenantAdminNamespaceAnnotation] == "" {
				log.Error(err, "Could not find tenant admin namespace key in annotation",
					"namespace", object.GetName())
				return nil
			}

			// Match found - add a Request for the tenantnamespace object
			result = append(result, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: annotations[TenantAdminNamespaceAnnotation],
				Name:      ref.Name,
			}})
		}
	}
	return result
}

func (e *enqueueTenantNamespace) InjectScheme(s *runtime.Scheme) error {
	kinds, _, err := s.ObjectKinds(ownerType)
	e.groupKind = schema.GroupKind{Group: kinds[0].Group, Kind: kinds[0].Kind}
	return err
}
