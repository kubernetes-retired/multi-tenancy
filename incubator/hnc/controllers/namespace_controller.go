/*

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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// NamespaceReconciler simply ensures that every namespace has a Hierarchy singleton.
//
// TODO: possibly copy the parent from the singleton to a label on the namespace.
type NamespaceReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update;patch

func (r *NamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("namespace", req.NamespacedName.Name)

	// Check to see if it's been deleted
	instance := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if !errors.IsNotFound(err) {
			log.Info("Couldn't read namespace")
			return ctrl.Result{}, err
		}

		return r.onDelete(log, req.NamespacedName.Name)
	}

	// If it hasn't been deleted, but it's *being* deleted, don't do anything.
	if !instance.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Update
	return r.onUpdate(ctx, log, instance.ObjectMeta.Name)
}

func (r *NamespaceReconciler) onUpdate(ctx context.Context, log logr.Logger, nm string) (ctrl.Result, error) {
	// Ensure the Hierarchy object exists and create it if it doesn't
	hier := &tenancy.Hierarchy{}
	hierName := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	err := r.Get(ctx, hierName, hier)
	switch {
	case err == nil:
		log.V(1).Info("singleton already exists")

	case errors.IsNotFound(err):
		// It doesn't exist; create it
		hier.ObjectMeta.Name = tenancy.Singleton
		hier.ObjectMeta.Namespace = nm
		if err := r.Create(ctx, hier); err != nil {
			log.Info("Couldn't create singleton")
			return ctrl.Result{}, err
		}
		log.Info("Successfully created singleton")

	default:
		log.Info("Couldn't read singleton")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) onDelete(log logr.Logger, nsnm string) (ctrl.Result, error) {
	log.Info("Deleted")
	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}
