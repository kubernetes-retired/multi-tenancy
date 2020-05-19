package validators

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

func TestDeletingConfigObject(t *testing.T) {
	t.Run("Delete config object", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := admission.Request{
			AdmissionRequest: v1beta1.AdmissionRequest{
				Operation: v1beta1.Delete,
				Name:      "config",
			}}
		config := &HNCConfig{}

		got := config.Handle(context.Background(), req)

		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())
	})
}

func TestDeletingOtherObject(t *testing.T) {
	t.Run("Delete config object", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := admission.Request{
			AdmissionRequest: v1beta1.AdmissionRequest{
				Operation: v1beta1.Delete,
				Name:      "other",
			}}
		config := &HNCConfig{}

		got := config.Handle(context.Background(), req)

		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())
	})
}

func TestInvalidName(t *testing.T) {
	t.Run("Invalid config name", func(t *testing.T) {
		g := NewGomegaWithT(t)
		c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: []api.TypeSynchronizationSpec{
			{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
			{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
		}}}
		// Name should be "config"
		c.Name = "invalid-name"
		config := &HNCConfig{}

		got := config.handle(context.Background(), c)

		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())
	})
}

func TestRBACTypes(t *testing.T) {
	config := &HNCConfig{}

	tests := []struct {
		name    string
		configs []api.TypeSynchronizationSpec
		allow   bool
	}{
		{
			name: "Correct RBAC config with propagate mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
			},
			allow: true,
		},
		{
			name: "Correct RBAC config with unset mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
			},
			allow: true,
		},
		{
			name: "Missing Role",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
			},
			allow: false,
		}, {
			name: "Missing RoleBinding",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
			},
			allow: false,
		}, {
			name: "Incorrect Role mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "ignore"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
			},
			allow: false,
		}, {
			name: "Incorrect RoleBinding mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "ignore"},
			},
			allow: false,
		}, {
			name: "Duplicate RBAC types with different modes",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
			},
			allow: false,
		},
		{
			name: "Duplicate RBAC types with the same mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
			},
			allow: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: tc.configs}}
			c.Name = "config"

			got := config.handle(context.Background(), c)

			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).Should(Equal(tc.allow))
		})
	}
}

func TestNonRBACTypes(t *testing.T) {
	f := fakeGVKValidator{"CronTab"}
	tests := []struct {
		name      string
		configs   []api.TypeSynchronizationSpec
		validator fakeGVKValidator
		allow     bool
	}{
		{
			name: "Correct Non-RBAC types config",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "ignore"},
				{APIVersion: "v1", Kind: "ResourceQuota"},
			},
			validator: f,
			allow:     true,
		},
		{
			name: "Resource does not exist",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
				// "CronTab" kind does not exist in "v1"
				{APIVersion: "v1", Kind: "CronTab", Mode: "ignore"},
			},
			validator: f,
			allow:     false,
		},
		{
			name: "Unrecognized mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
				// "delete" mode is unsupported
				{APIVersion: "v1", Kind: "Secret", Mode: "delete"},
			},
			validator: f,
			allow:     false,
		}, {
			name: "Duplicate types with different modes",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "ignore"},
				{APIVersion: "v1", Kind: "Secret", Mode: "propagate"},
			},
			validator: f,
			allow:     false,
		}, {
			name: "Duplicate types with the same mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "ignore"},
				{APIVersion: "v1", Kind: "Secret", Mode: "ignore"},
			},
			validator: f,
			allow:     false,
		}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: tc.configs}}
			c.Name = "config"
			config := &HNCConfig{validator: tc.validator}

			got := config.handle(context.Background(), c)

			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).Should(Equal(tc.allow))
		})
	}
}

// fakeGVKValidator implements gvkValidator. Any kind that are in the slice are denied; anything else
// is allowed.
type fakeGVKValidator []string

func (f fakeGVKValidator) Exists(_ context.Context, gvk schema.GroupVersionKind) error {
	for _, k := range f {
		if k == gvk.Kind {
			return fmt.Errorf("%s does not exist", gvk)
		}
	}
	return nil
}
