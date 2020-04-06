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

package controllers

import (
	"context"
	"fmt"

	tenantutil "github.com/kubernetes-sigs/multi-tenancy/tenant/controllers/util"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/api/v1alpha1"
)

const (
	// TenantAdminNamespaceAnnotation is the key for tenantAdminNamespace annotation
	TenantAdminNamespaceAnnotation = "x-k8s.io/tenantAdminNamespace"

	TenantNamespaceFinalizer = "tenantnamespace.finalizer.x-k8s.io"
)

// TenantNamespaceReconciler reconciles a TenantNamespace object
type TenantNamespaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenantnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenantnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create

func (r *TenantNamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("tenantnamespace", req.NamespacedName)

	// Fetch the TenantNamespace instance
	instance := &tenancyv1alpha1.TenantNamespace{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Fetch namespace list
	nsList := &corev1.NamespaceList{}
	err = r.List(ctx, nsList)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch tenant list
	tenantList := &tenancyv1alpha1.TenantList{}
	err = r.List(ctx, tenantList)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Find the tenant of this instance
	requireNamespacePrefix := false
	var tenantName string
	for _, each := range tenantList.Items {
		if each.Spec.TenantAdminNamespaceName == instance.Namespace {
			requireNamespacePrefix = each.Spec.RequireNamespacePrefix
			tenantName = each.Name
			break
		}
	}
	if tenantName == "" {
		err = fmt.Errorf("TenantNamespace CR %v does not belong to any tenant", instance)
		return ctrl.Result{}, err

	}

	// Handle tenantNamespace CR deletion
	if instance.DeletionTimestamp != nil {
		// Remove namespace from tenant clusterrole
		if err = r.updateTenantClusterRole(ctx, log, tenantName, instance.Status.OwnedNamespace, false); err != nil {
			return ctrl.Result{}, err
		} else {
			instanceClone := instance.DeepCopy()
			if containsString(instanceClone.Finalizers, TenantNamespaceFinalizer) {
				instanceClone.Finalizers = removeString(instanceClone.Finalizers, TenantNamespaceFinalizer)
			}
			err = r.Update(ctx, instanceClone)
			return ctrl.Result{}, err
		}
	}

	// In case namespace already exists
	tenantNsName := tenantutil.GetTenantNamespaceName(requireNamespacePrefix, instance)
	expectedOwnerRef := metav1.OwnerReference{
		APIVersion: tenancyv1alpha1.GroupVersion.String(),
		Kind:       "TenantNamespace",
		Name:       instance.Name,
		UID:        instance.UID,
	}

	found := false
	for _, each := range nsList.Items {
		if each.Name == tenantNsName {
			found = true
			// Check OwnerReference
			isOwner := false
			for _, ownerRef := range each.OwnerReferences {
				if ownerRef == expectedOwnerRef {
					isOwner = true
					break
				} else if ownerRef.APIVersion == expectedOwnerRef.APIVersion && ownerRef.Kind == expectedOwnerRef.Kind {
					// The namespace is owned by another TenantNamespace CR, fail the reconcile
					err = fmt.Errorf("Namespace %v is owned by another %v TenantNamespace CR", each.Name, ownerRef)
					return ctrl.Result{}, err
				}
			}
			if !isOwner {
				log.Info("Namespace has been created without TenantNamespace owner", "namespace", each.Name)
				// Obtain namespace ownership by setting ownerReference, and add annotation
				if err = r.updateNamespace(log, &each, &instance.Namespace, &expectedOwnerRef); err != nil {
					return ctrl.Result{}, err
				}
			}
			break
		}
	}

	// In case a new namespace needs to be created
	if !found {
		tenantNs := &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: tenantNsName,
				Annotations: map[string]string{
					TenantAdminNamespaceAnnotation: instance.Namespace,
				},
				OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
			},
		}
		if err = r.Client.Create(ctx, tenantNs); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status
	instanceClone := instance.DeepCopy()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !containsString(instanceClone.Finalizers, TenantNamespaceFinalizer) {
			instanceClone.Finalizers = append(instanceClone.Finalizers, TenantNamespaceFinalizer)
		}
		instanceClone.Status.OwnedNamespace = tenantNsName
		updateErr := r.Update(ctx, instanceClone)
		if updateErr == nil {
			return nil
		}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instanceClone); err != nil {
			log.Info("Fail to fetch tenantNamespace CR on update", "tenantNamespace", instance.Name)
		}
		return updateErr
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Add namespace to tenant clusterrule to allow tenant admins to access it.
	err = r.updateTenantClusterRole(ctx, log, tenantName, tenantNsName, true)

	return ctrl.Result{}, nil
}

func (r *TenantNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancyv1alpha1.TenantNamespace{}).
		Owns(&corev1.Namespace{}).
		Complete(r)
}

// This method updates tenant clusterrule to add or remove the tenant namespace.
func (r *TenantNamespaceReconciler) updateTenantClusterRole(ctx context.Context, log logr.Logger, tenantName, tenantNsName string, add bool) error {
	var err error
	cr := &rbacv1.ClusterRole{}
	if err = r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-tenant-admin-role", tenantName)}, cr); err != nil {
		return err
	}
	cr = cr.DeepCopy()
	foundNsRule := false
	needUpdate := add
	for i, each := range cr.Rules {
		for _, resource := range each.Resources {
			if resource == "namespaces" {
				foundNsRule = true
				break
			}
		}
		if foundNsRule {
			idx := 0
			for ; idx < len(each.ResourceNames); idx++ {
				if each.ResourceNames[idx] == tenantNsName {
					needUpdate = !add
					break
				}
			}
			if needUpdate {
				if add {
					cr.Rules[i].ResourceNames = append(cr.Rules[i].ResourceNames, tenantNsName)
				} else {
					cr.Rules[i].ResourceNames = append(cr.Rules[i].ResourceNames[:idx], cr.Rules[i].ResourceNames[idx+1:]...)
				}
			}
			break
		}
	}
	if !foundNsRule {
		return fmt.Errorf("Cluster Role %s-tenant-admin-role does not have rules for namespaces.", tenantName)
	}
	if needUpdate {
		crClone := cr.DeepCopy()
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			crClone.Rules = cr.Rules
			updateErr := r.Update(ctx, crClone)
			if updateErr == nil {
				return nil
			}
			if err := r.Get(ctx, types.NamespacedName{Name: crClone.Name}, crClone); err != nil {
				log.Info("Fail to fetch clusterrole on update", "clusterrole", crClone.Name)
			}
			return updateErr
		})
	}
	return err
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// Add a ownerReference and tenant admin namespace annotation to input namespace
func (r *TenantNamespaceReconciler) updateNamespace(log logr.Logger, ns *corev1.Namespace, tenantAdminNamespaceName *string, ownerRef *metav1.OwnerReference) error {
	nsClone := ns.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		nsClone.OwnerReferences = append(nsClone.OwnerReferences, *ownerRef)
		if nsClone.Annotations == nil {
			nsClone.Annotations = make(map[string]string)
		}
		nsClone.Annotations[TenantAdminNamespaceAnnotation] = *tenantAdminNamespaceName
		updateErr := r.Update(context.TODO(), nsClone)
		if updateErr == nil {
			return nil
		}
		key := types.NamespacedName{
			Name: nsClone.Name,
		}
		if err := r.Get(context.TODO(), key, nsClone); err != nil {
			log.Info("Fail to fetch namespace on update failure", "namespace", nsClone.Name)
		}
		return updateErr
	})
	return err
}
