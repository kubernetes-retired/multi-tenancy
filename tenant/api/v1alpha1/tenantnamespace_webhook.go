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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var tenantnamespacelog = logf.Log.WithName("tenantnamespace-resource")

func (r *TenantNamespace) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-tenancy-x-k8s-io-v1alpha1-tenantnamespace,mutating=true,failurePolicy=fail,groups=tenancy.x-k8s.io,resources=tenantnamespaces,verbs=create;update,versions=v1alpha1,name=mtenantnamespace.kb.io

var _ webhook.Defaulter = &TenantNamespace{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *TenantNamespace) Default() {
	tenantnamespacelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-tenancy-x-k8s-io-v1alpha1-tenantnamespace,mutating=false,failurePolicy=fail,groups=tenancy.x-k8s.io,resources=tenantnamespaces,versions=v1alpha1,name=vtenantnamespace.kb.io

var _ webhook.Validator = &TenantNamespace{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *TenantNamespace) ValidateCreate() error {
	tenantnamespacelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *TenantNamespace) ValidateUpdate(old runtime.Object) error {
	oldobj := old.(*TenantNamespace)
	tenantnamespacelog.Info("validate update", "name", r.Name)
	allErrs := field.ErrorList{}
	if r.Spec.Name != oldobj.Spec.Name {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("name"), fmt.Sprintf("cannot modify the name field in spec after initial creation (attempting to change from %s to %s)", oldobj.Spec.Name, r.Spec.Name)))
	}
	return allErrs.ToAggregate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *TenantNamespace) ValidateDelete() error {
	tenantnamespacelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
