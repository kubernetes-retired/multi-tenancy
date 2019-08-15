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
	"context"

	tenancyv1alpha1 "github.com/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new TenantNamespace Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTenantNamespace{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("tenantnamespace-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to TenantNamespace
	err = c.Watch(&source.Kind{Type: &tenancyv1alpha1.TenantNamespace{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileTenantNamespace{}

// ReconcileTenantNamespace reconciles a TenantNamespace object
type ReconcileTenantNamespace struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a TenantNamespace object and makes changes based on the state read
// and what is in the TenantNamespace.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenantnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenantnamespaces/status,verbs=get;update;patch
func (r *ReconcileTenantNamespace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the TenantNamespace instance
	instance := &tenancyv1alpha1.TenantNamespace{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
