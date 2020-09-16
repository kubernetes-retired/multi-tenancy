package validators

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/foresttest"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

func TestDeletingConfigObject(t *testing.T) {
	t.Run("Delete config object", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := admission.Request{
			AdmissionRequest: v1beta1.AdmissionRequest{
				Operation: v1beta1.Delete,
				Name:      api.HNCConfigSingleton,
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

func TestRBACTypes(t *testing.T) {
	f := forest.NewForest()
	config := &HNCConfig{Forest: f}

	tests := []struct {
		name    string
		configs []api.TypeSynchronizationSpec
		allow   bool
	}{
		{
			name: "Correct RBAC config with Propagate mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
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
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
			},
			allow: false,
		}, {
			name: "Missing RoleBinding",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
			},
			allow: false,
		}, {
			name: "Incorrect Role mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Ignore"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
			},
			allow: false,
		}, {
			name: "Incorrect RoleBinding mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Ignore"},
			},
			allow: false,
		}, {
			name: "Duplicate RBAC types with different modes",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
			},
			allow: false,
		},
		{
			name: "Duplicate RBAC types with the same mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
			},
			allow: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: tc.configs}}
			c.Name = api.HNCConfigSingleton

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
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Ignore"},
				{APIVersion: "v1", Kind: "ResourceQuota"},
			},
			validator: f,
			allow:     true,
		},
		{
			name: "Resource does not exist",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
				// "CronTab" kind does not exist in "v1"
				{APIVersion: "v1", Kind: "CronTab", Mode: "Ignore"},
			},
			validator: f,
			allow:     false,
		}, {
			name: "Duplicate types with different modes",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Ignore"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Propagate"},
			},
			validator: f,
			allow:     false,
		}, {
			name: "Duplicate types with the same mode",
			configs: []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Ignore"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Ignore"},
			},
			validator: f,
			allow:     false,
		}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: tc.configs}}
			c.Name = api.HNCConfigSingleton
			config := &HNCConfig{validator: tc.validator, Forest: forest.NewForest()}

			got := config.handle(context.Background(), c)

			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).Should(Equal(tc.allow))
		})
	}
}

func TestPropagateConflict(t *testing.T) {
	tests := []struct {
		name         string
		inNamespaces string
		allow        bool
	}{{
		name:         "Objects with the same name existing in namespaces that one is not an ancestor of the other would not cause overwriting conflict",
		inNamespaces: "bc",
		allow:        true,
	}, {
		name:         "Objects with the same name existing in namespaces that one is an ancestor of the other would have overwriting conflict",
		inNamespaces: "ab",
		allow:        false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			configs := []api.TypeSynchronizationSpec{
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: "Propagate"},
				{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: "Propagate"},
				{APIVersion: "v1", Kind: "Secret", Mode: "Propagate"}}
			c := &api.HNCConfiguration{Spec: api.HNCConfigurationSpec{Types: configs}}
			c.Name = api.HNCConfigSingleton
			// Create a forest with "a" as the parent and "b" and "c" as the children.
			f := foresttest.Create("-aa")
			config := &HNCConfig{validator: fakeGVKValidator{}, Forest: f}

			// Add source objects to the forest.
			inst := &unstructured.Unstructured{}
			inst.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
			inst.SetName("my-creds")
			for _, c := range tc.inNamespaces {
				f.Get(string(c)).SetSourceObject(inst)
			}
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
