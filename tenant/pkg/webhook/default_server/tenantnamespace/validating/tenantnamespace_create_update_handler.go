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
	"net/http"

	tenancyv1alpha1 "github.com/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "validating-create-update-tenantnamespace"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &TenantNamespaceCreateUpdateHandler{})
}

// TenantNamespaceCreateUpdateHandler handles TenantNamespace
type TenantNamespaceCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *TenantNamespaceCreateUpdateHandler) validateTenantNamespaceUpdate(obj *tenancyv1alpha1.TenantNamespace, oldobj *tenancyv1alpha1.TenantNamespace) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldobj.ObjectMeta, field.NewPath("metadata"))
	if obj.Spec.Name != oldobj.Spec.Name {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("name"), "update to name field in spec is forbiddern"))
	}
	return allErrs
}

func validateTenantNamespaceName(name string, prefix bool) []string {
	// We don't have name requirement for now
	return nil
}
func (h *TenantNamespaceCreateUpdateHandler) validateTenantNamespaceCreate(obj *tenancyv1alpha1.TenantNamespace) field.ErrorList {
	path := field.NewPath("metadata")
	allErrs := apivalidation.ValidateObjectMeta(&obj.ObjectMeta, true, validateTenantNamespaceName, path)

	// Fetch tenant list
	tenantList := &tenancyv1alpha1.TenantList{}
	err := h.Client.List(context.TODO(), &client.ListOptions{}, tenantList)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(path.Child("Namespace"), obj.Namespace, "cannot validate namespace because cannot get tenant list"))
	}
	foundTenant := false
	for _, each := range tenantList.Items {
		if each.Spec.TenantAdminNamespaceName == obj.Namespace {
			foundTenant = true
			break
		}
	}
	if !foundTenant {
		allErrs = append(allErrs, field.Invalid(path.Child("Namespace"), obj.Namespace, "namespace has to be a tenant admin namespace"))
	}
	return allErrs
}

var _ admission.Handler = &TenantNamespaceCreateUpdateHandler{}

// Handle handles admission requests.
func (h *TenantNamespaceCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &tenancyv1alpha1.TenantNamespace{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if createErrorList := h.validateTenantNamespaceCreate(obj); len(createErrorList) > 0 {
			return admission.ErrorResponse(http.StatusUnprocessableEntity, createErrorList.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldobj := &tenancyv1alpha1.TenantNamespace{}
		if err := h.Decoder.Decode(types.Request{
			AdmissionRequest: &admissionv1beta1.AdmissionRequest{Object: req.AdmissionRequest.OldObject},
		}, oldobj); err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}
		if updateErrorList := h.validateTenantNamespaceUpdate(obj, oldobj); len(updateErrorList) > 0 {
			return admission.ErrorResponse(http.StatusInternalServerError, updateErrorList.ToAggregate())
		}
	}
	return admission.ValidationResponse(true, "")
}

var _ inject.Client = &TenantNamespaceCreateUpdateHandler{}

// InjectClient injects the client into the TenantNamespaceCreateUpdateHandler
func (h *TenantNamespaceCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ inject.Decoder = &TenantNamespaceCreateUpdateHandler{}

// InjectDecoder injects the decoder into the TenantNamespaceCreateUpdateHandler
func (h *TenantNamespaceCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
