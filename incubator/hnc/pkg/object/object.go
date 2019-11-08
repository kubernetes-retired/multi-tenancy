package object

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// metaPrefix returns the prefix (if any) of a label key.
// Reference https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
func metaPrefix(s string) string {
	p := strings.Split(s, "/")
	if len(p) == 1 {
		return ""
	}
	return p[0]
}

// Canonical returns a canonicalized version of the object - that is, one that has the same name,
// spec and non-HNC labels and annotations, but with the status and all other metadata cleared
// (including, notably, the namespace). The resulting object is suitable to be copied into a new
// namespace, or two canonicalized objects are suitable for being compared via reflect.DeepEqual.
//
// As a side effect, the label and annotation maps are always initialized in the returned value.
func Canonical(inst *unstructured.Unstructured) *unstructured.Unstructured {
	// Start with a copy and clear the status and metadata
	c := inst.DeepCopy()
	delete(c.Object, "status")
	delete(c.Object, "metadata")

	// Restore the whitelisted metadata. Name:
	c.SetName(inst.GetName())

	// Non-HNC annotations:
	newAnnots := map[string]string{}
	for k, v := range inst.GetAnnotations() {
		if !strings.HasSuffix(metaPrefix(k), api.MetaGroup) {
			newAnnots[k] = v
		}
	}
	c.SetAnnotations(newAnnots)

	// Non-HNC labels:
	newLabels := map[string]string{}
	for k, v := range inst.GetLabels() {
		if !strings.HasSuffix(metaPrefix(k), api.MetaGroup) {
			newLabels[k] = v
		}
	}
	c.SetLabels(newLabels)

	return c
}
