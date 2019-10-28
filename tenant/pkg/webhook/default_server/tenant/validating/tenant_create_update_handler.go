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

package validating

import (
	"context"
	"fmt"
	"net/http"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "validating-create-update-tenant"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &TenantCreateUpdateHandler{})
}

// TenantCreateUpdateHandler handles Tenant
type TenantCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *TenantCreateUpdateHandler) validateTenantUpdate(obj *tenancyv1alpha1.Tenant, oldobj *tenancyv1alpha1.Tenant) field.ErrorList {
	allErrs := field.ErrorList{}
	if obj.Spec.TenantAdminNamespaceName != oldobj.Spec.TenantAdminNamespaceName {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tenantAdminNamespaceName"), fmt.Sprintf("cannot modify the tenantAdminNamespaceName field in spec after initial creation (attempting to change from %s to %s)", oldobj.Spec.TenantAdminNamespaceName, obj.Spec.TenantAdminNamespaceName)))
	}
	if obj.Spec.RequireNamespacePrefix != oldobj.Spec.RequireNamespacePrefix {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("requireNamespacePrefix"), fmt.Sprintf("cannot modify the requireNamespacePrefix field in spec after initial creation (attempting to change from %v to %v)", oldobj.Spec.RequireNamespacePrefix, obj.Spec.RequireNamespacePrefix)))
	}
	return allErrs
}

func (h *TenantCreateUpdateHandler) validateTenantCreate(obj *tenancyv1alpha1.Tenant) field.ErrorList {
	allErrs := field.ErrorList{}
	for _, msg := range apivalidation.ValidateNamespaceName(obj.Spec.TenantAdminNamespaceName, false) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tenantAdminNamespaceName"), obj.Spec.TenantAdminNamespaceName, msg))
	}
	return allErrs
}

var _ admission.Handler = &TenantCreateUpdateHandler{}

// Handle handles admission requests.
func (h *TenantCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &tenancyv1alpha1.Tenant{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if createErrorList := h.validateTenantCreate(obj); len(createErrorList) > 0 {
			return admission.ErrorResponse(http.StatusUnprocessableEntity, createErrorList.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldobj := &tenancyv1alpha1.Tenant{}
		if err := h.Decoder.Decode(types.Request{
			AdmissionRequest: &admissionv1beta1.AdmissionRequest{Object: req.AdmissionRequest.OldObject},
		}, oldobj); err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}
		if updateErrorList := h.validateTenantUpdate(obj, oldobj); len(updateErrorList) > 0 {
			return admission.ErrorResponse(http.StatusInternalServerError, updateErrorList.ToAggregate())
		}
	}
	return admission.ValidationResponse(true, "")
}

//var _ inject.Client = &TenantCreateUpdateHandler{}
//
//// InjectClient injects the client into the TenantCreateUpdateHandler
//func (h *TenantCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &TenantCreateUpdateHandler{}

// InjectDecoder injects the decoder into the TenantCreateUpdateHandler
func (h *TenantCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
