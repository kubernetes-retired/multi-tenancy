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
	"fmt"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	tenantutil "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/util"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

const (
	// TenantAdminNamespaceAnnotation is the key for tenantAdminNamespace annotation
	TenantAdminNamespaceAnnotation = "x-k8s.io/tenantAdminNamespace"

	TenantNamespaceFinalizer = "tenantnamespace.finalizer.x-k8s.io"
)

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

	// Watch for changes to namespaces
	err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &enqueueTenantNamespace{})
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

// Add a ownerReference and tenant admin namespace annotation to input namespace
func (r *ReconcileTenantNamespace) updateNamespace(ns *corev1.Namespace, tenantAdminNamespaceName *string, ownerRef *metav1.OwnerReference) error {
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

func findTenant(adminNsName string, tList *tenancyv1alpha1.TenantList) (string, bool) {
	// Find the tenant that owns the adminNs
	requireNamespacePrefix := false
	var tenantName string
	for _, each := range tList.Items {
		if each.Spec.TenantAdminNamespaceName == adminNsName {
			requireNamespacePrefix = each.Spec.RequireNamespacePrefix
			tenantName = each.Name
			break
		}
	}
	return tenantName, requireNamespacePrefix
}

// Reconcile reads that state of the cluster for a TenantNamespace object and makes changes based on the state read
// and what is in the TenantNamespace.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create
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
	// Fetch namespace list
	nsList := &corev1.NamespaceList{}
	err = r.List(context.TODO(), &client.ListOptions{}, nsList)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch tenant list
	tenantList := &tenancyv1alpha1.TenantList{}
	err = r.List(context.TODO(), &client.ListOptions{}, tenantList)
	if err != nil {
		return reconcile.Result{}, err
	}

	tenantName, requireNamespacePrefix := findTenant(instance.Namespace, tenantList)

	// Handle tenantNamespace CR deletion
	if instance.DeletionTimestamp != nil {
		if tenantName != "" {
			// Remove namespace from tenant clusterrole
			if err = r.updateTenantClusterRole(tenantName, instance.Status.OwnedNamespace, false); err != nil {
				return reconcile.Result{}, err
			}
		}
		instanceClone := instance.DeepCopy()
		if containsString(instanceClone.Finalizers, TenantNamespaceFinalizer) {
			instanceClone.Finalizers = removeString(instanceClone.Finalizers, TenantNamespaceFinalizer)
		}
		err = r.Update(context.TODO(), instanceClone)
		return reconcile.Result{}, err

	} else if tenantName == "" {
		return reconcile.Result{}, fmt.Errorf("TenantNamespace CR %v does not belong to any tenant", instance)
	}

	// In case namespace already exists
	tenantNsName := tenantutil.GetTenantNamespaceName(requireNamespacePrefix, instance)
	expectedOwnerRef := metav1.OwnerReference{
		APIVersion: tenancyv1alpha1.SchemeGroupVersion.String(),
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
					return reconcile.Result{}, err
				}
			}
			if !isOwner {
				log.Info("Namespace has been created without TenantNamespace owner", "namespace", each.Name)
				// Obtain namespace ownership by setting ownerReference, and add annotation
				if err = r.updateNamespace(&each, &instance.Namespace, &expectedOwnerRef); err != nil {
					return reconcile.Result{}, err
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
		if err = r.Client.Create(context.TODO(), tenantNs); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Update status
	instanceClone := instance.DeepCopy()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !containsString(instanceClone.Finalizers, TenantNamespaceFinalizer) {
			instanceClone.Finalizers = append(instanceClone.Finalizers, TenantNamespaceFinalizer)
		}
		instanceClone.Status.OwnedNamespace = tenantNsName
		updateErr := r.Update(context.TODO(), instanceClone)
		if updateErr == nil {
			return nil
		}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instanceClone); err != nil {
			log.Info("Fail to fetch tenantNamespace CR on update", "tenantNamespace", instance.Name)
		}
		return updateErr
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	// Add namespace to tenant clusterrule to allow tenant admins to access it.
	err = r.updateTenantClusterRole(tenantName, tenantNsName, true)
	return reconcile.Result{}, err
}

// This method updates tenant clusterrule to add or remove the tenant namespace.
func (r *ReconcileTenantNamespace) updateTenantClusterRole(tenantName, tenantNsName string, add bool) error {
	var err error
	cr := &rbacv1.ClusterRole{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-tenant-admin-role", tenantName)}, cr); err != nil {
		if errors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
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
			updateErr := r.Update(context.TODO(), crClone)
			if updateErr == nil {
				return nil
			}
			if err := r.Get(context.TODO(), types.NamespacedName{Name: crClone.Name}, crClone); err != nil {
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
