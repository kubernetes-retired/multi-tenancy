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

package v1alpha1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var vclog = logf.Log.WithName("virtualcluster-webhook")

func (vc *VirtualCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	vclog.Info("setup virtualcluster validation webhook")
	return ctrl.NewWebhookManagedBy(mgr).
		For(vc).
		Complete()
}

var _ webhook.Validator = &VirtualCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (vc *VirtualCluster) ValidateCreate() error {
	vclog.Info("validate create", "vc-name", vc.Name)
	// do nothing for delete request
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (new *VirtualCluster) ValidateUpdate(old runtime.Object) error {
	vclog.Info("validate update", "vc-name", new.Name)
	return new.validateVirtualClusterUpdate(old)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (vc *VirtualCluster) ValidateDelete() error {
	vclog.Info("validate delete", "vc-name", vc.Name)
	// do nothing for delete request
	return nil
}

func (vc *VirtualCluster) validateVirtualClusterUpdate(old runtime.Object) error {
	var allErrs field.ErrorList
	oldVC, ok := old.(*VirtualCluster)
	if !ok {
		return errors.New("fail to assert runtime.Object to tenancyv1alpha1.VirtualCluster")
	}
	// once the VC.Status.Phase is set, it can't be set to empty again
	if oldVC.Status.Phase != "" && vc.Status.Phase == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("status").Child("phase"),
				vc.Name, "cannot set virtualcluster.Status.Phase to empty"))
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "tenancy.x-k8s.io", Kind: "VirtualCluster"},
			vc.Name, allErrs)
	}
	return nil
}
