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

package v1alpha1

import (
	"fmt"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var tenantlog = logf.Log.WithName("tenant-resource")

func (r *Tenant) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:verbs=create;update,path=/validate-tenancy-x-k8s-io-v1alpha1-tenant,mutating=false,failurePolicy=fail,groups=tenancy.x-k8s.io,resources=tenants,versions=v1alpha1,name=vtenant.kb.io
var _ webhook.Validator = &Tenant{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Tenant) ValidateCreate() error {
	tenantlog.Info("validate create", "name", r.Name)

	allErrs := field.ErrorList{}
	for _, msg := range apivalidation.ValidateNamespaceName(r.Spec.TenantAdminNamespaceName, false) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tenantAdminNamespaceName"), r.Spec.TenantAdminNamespaceName, msg))
	}
	return allErrs.ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Tenant) ValidateUpdate(old runtime.Object) error {
	oldObj := old.(*Tenant)
	tenantlog.Info("validate update", "name", r.Name)
	allErrs := field.ErrorList{}
	if r.Spec.TenantAdminNamespaceName != oldObj.Spec.TenantAdminNamespaceName {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tenantAdminNamespaceName"), fmt.Sprintf("cannot modify the tenantAdminNamespaceName field in spec after initial creation (attempting to change from %s to %s)", oldObj.Spec.TenantAdminNamespaceName, r.Spec.TenantAdminNamespaceName)))
	}
	if r.Spec.RequireNamespacePrefix != oldObj.Spec.RequireNamespacePrefix {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("requireNamespacePrefix"), fmt.Sprintf("cannot modify the requireNamespacePrefix field in spec after initial creation (attempting to change from %v to %v)", oldObj.Spec.RequireNamespacePrefix, r.Spec.RequireNamespacePrefix)))
	}
	return allErrs.ToAggregate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Tenant) ValidateDelete() error {
	tenantnamespacelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
