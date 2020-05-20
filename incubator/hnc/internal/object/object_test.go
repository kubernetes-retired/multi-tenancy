package object

import (
	. "github.com/onsi/gomega"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

func TestCanonical(t *testing.T) {
	tests := []struct {
		name     string
		inst     *unstructured.Unstructured
		expected *unstructured.Unstructured
	}{{
		name: "Non-HNC metadata should be kept",
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"label": "value",
					},
					"annotations": map[string]interface{}{
						"annotation": "example",
					},
				},
			},
		},
	}, {
		name: "HNC metadata should be stripped",
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
						// tree label
						"foo.tree." + api.MetaGroup + "/depth": "1",
					},
					"annotations": map[string]interface{}{
						api.MetaGroup + "/annotation": "value",
					},
				},
			},
		},
		expected: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels":      map[string]interface{}{},
					"annotations": map[string]interface{}{},
				},
			},
		},
	}, {
		name: "HNC-alike metadata (unlikely) should be kept",
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"label" + api.MetaGroup: "value",
					},
					"annotations": map[string]interface{}{
						"It is unlikely to contain " + api.MetaGroup + " for non-HNC annotations": "example",
					},
				},
			},
		},
	},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			// Test
			got := Canonical(tc.inst)
			if tc.expected == nil {
				tc.expected = tc.inst
			}
			// Report
			g.Expect(got).Should(Equal(tc.expected))
		})
	}
}
