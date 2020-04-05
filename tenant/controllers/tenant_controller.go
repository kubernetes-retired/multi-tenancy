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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/api/v1alpha1"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;create;update;patch

func (r *TenantReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("tenant", req.NamespacedName)
	// Fetch the Tenant instance
	instance := &tenancyv1alpha1.Tenant{}
	// Tenant is a cluster scoped CR, we should clear the namespace field in request
	req.NamespacedName.Namespace = ""
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

	expectedOwnerRef := metav1.OwnerReference{
		APIVersion: tenancyv1alpha1.GroupVersion.String(),
		Kind:       "Tenant",
		Name:       instance.Name,
		UID:        instance.UID,
	}

	// Create tenantAdminNamespace
	if instance.Spec.TenantAdminNamespaceName != "" {
		nsList := &corev1.NamespaceList{}
		err := r.List(context.TODO(), nsList)
		if err != nil {
			return ctrl.Result{}, err
		}
		foundNs := false
		for _, each := range nsList.Items {
			if each.Name == instance.Spec.TenantAdminNamespaceName {
				foundNs = true
				// Check OwnerReference
				isOwner := false
				for _, ownerRef := range each.OwnerReferences {
					if ownerRef == expectedOwnerRef {
						isOwner = true
						break
					}
				}
				if !isOwner {
					err = fmt.Errorf("TenantAdminNamespace %v is owned by %v", each.Name, each.OwnerReferences)
					return ctrl.Result{}, err
				}
				break
			}
		}
		if !foundNs {
			tenantAdminNs := &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            instance.Spec.TenantAdminNamespaceName,
					OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
				},
			}
			if err := r.Client.Create(context.TODO(), tenantAdminNs); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	// Create RBACs for tenantAdmins.
	if instance.Spec.TenantAdmins != nil {
		// First, create cluster roles to allow them to access tenant CR and tenantAdminNamespace.
		crName := fmt.Sprintf("%s-tenant-admin-role", instance.Name)
		cr := &rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            crName,
				OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:         []string{"get", "list", "watch", "update", "patch", "delete"},
					APIGroups:     []string{tenancyv1alpha1.GroupVersion.Group},
					Resources:     []string{"tenants"},
					ResourceNames: []string{instance.Name},
				},
				{
					Verbs:         []string{"get", "list", "watch"},
					APIGroups:     []string{""},
					Resources:     []string{"namespaces"},
					ResourceNames: []string{instance.Spec.TenantAdminNamespaceName},
				},
			},
		}
		if err := r.clientApply(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		crbindingName := fmt.Sprintf("%s-tenant-admins-rolebinding", instance.Name)
		crbinding := &rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRoleBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            crbindingName,
				OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
			},
			Subjects: instance.Spec.TenantAdmins,
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     crName,
			},
		}
		if err = r.clientApply(ctx, crbinding); err != nil {
			return ctrl.Result{}, err
		}
		// Second, create namespace role to allow them to create tenantnamespace CR in tenantAdminNamespace.
		role := &rbacv1.Role{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "Role",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "tenant-admin-role",
				Namespace:       instance.Spec.TenantAdminNamespaceName,
				OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{tenancyv1alpha1.GroupVersion.Group},
					Resources: []string{"tenantnamespaces"},
				},
			},
		}
		if err := r.clientApply(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
		rbinding := &rbacv1.RoleBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "RoleBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "tenant-admins-rolebinding",
				Namespace:       instance.Spec.TenantAdminNamespaceName,
				OwnerReferences: []metav1.OwnerReference{expectedOwnerRef},
			},
			Subjects: instance.Spec.TenantAdmins,
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     "tenant-admin-role",
			},
		}
		if err = r.clientApply(ctx, rbinding); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil

}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancyv1alpha1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Complete(r)
}

// Create if not existing, update otherwise
func (r *TenantReconciler) clientApply(ctx context.Context, obj runtime.Object) error {
	var err error
	if err = r.Client.Create(ctx, obj); err != nil {
		if errors.IsAlreadyExists(err) {
			// Update instead of create
			err = r.Client.Update(ctx, obj)
		}
	}
	return err
}
