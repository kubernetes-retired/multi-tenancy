package reconcilers_test

import (
	"context"
	"crypto/rand"
	"fmt"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
)

// GVKs maps a kind to its corresponding GVK.
var GVKs = map[string]schema.GroupVersionKind{
	"Secret":        {Group: "", Version: "v1", Kind: "Secret"},
	"Role":          {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	"RoleBinding":   {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	"NetworkPolicy": {Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
	"ResourceQuota": {Group: "", Version: "v1", Kind: "ResourceQuota"},
	"LimitRange":    {Group: "", Version: "v1", Kind: "LimitRange"},
	"ConfigMap":     {Group: "", Version: "v1", Kind: "ConfigMap"},
	// CronTab is a custom resource.
	"CronTab": {Group: "stable.example.com", Version: "v1", Kind: "CronTab"},
}

func setParent(ctx context.Context, nm string, pnm string) {
	hier := newOrGetHierarchy(ctx, nm)
	oldPNM := hier.Spec.Parent
	hier.Spec.Parent = pnm
	updateHierarchy(ctx, hier)
	if oldPNM != "" {
		EventuallyWithOffset(1, func() []string {
			pHier := getHierarchyWithOffset(1, ctx, oldPNM)
			return pHier.Status.Children
		}).ShouldNot(ContainElement(nm))
	}
	if pnm != "" {
		EventuallyWithOffset(1, func() []string {
			pHier := getHierarchyWithOffset(1, ctx, pnm)
			return pHier.Status.Children
		}).Should(ContainElement(nm))
	}
}

// createNS is a convenience function to create a namespace and wait for its singleton to be
// created. It's used in other tests in this package, but basically duplicates the code in this test
// (it didn't originally). TODO: refactor.
func createNS(ctx context.Context, prefix string) string {
	nm := createNSName(prefix)

	// Create the namespace
	ns := &corev1.Namespace{}
	ns.Name = nm
	Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	return nm
}

// createNSName generates random namespace names. Namespaces are never deleted in test-env because
// the building Namespace controller (which finalizes namespaces) doesn't run; I searched Github and
// found that everyone who was deleting namespaces was *also* very intentionally generating random
// names, so I guess this problem is widespread.
func createNSName(prefix string) string {
	suffix := make([]byte, 10)
	rand.Read(suffix)
	return fmt.Sprintf("%s-%x", prefix, suffix)
}

// createNSWithLabel has similar function to createNS with label as additional parameter
func createNSWithLabel(ctx context.Context, prefix string, label map[string]string) string {
	nm := createNSName(prefix)

	// Create the namespace
	ns := &corev1.Namespace{}
	ns.SetLabels(label)
	ns.Name = nm
	Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	return nm
}

func updateHNCConfig(ctx context.Context, c *api.HNCConfiguration) error {
	if c.CreationTimestamp.IsZero() {
		return k8sClient.Create(ctx, c)
	} else {
		return k8sClient.Update(ctx, c)
	}
}

func resetHNCConfigToDefault(ctx context.Context) error {
	c := getHNCConfig(ctx)
	c.Spec = config.GetDefaultConfigSpec()
	c.Status.Types = nil
	c.Status.Conditions = nil
	return k8sClient.Update(ctx, c)
}

func getHNCConfig(ctx context.Context) *api.HNCConfiguration {
	return getHNCConfigWithOffsetAndName(1, ctx, api.HNCConfigSingleton)
}

func getHNCConfigWithOffsetAndName(offset int, ctx context.Context, nm string) *api.HNCConfiguration {
	snm := types.NamespacedName{Name: nm}
	config := &api.HNCConfiguration{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, snm, config)
	}).Should(Succeed())
	return config
}

func addToHNCConfig(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	Eventually(func() error {
		c := getHNCConfig(ctx)
		spec := api.TypeSynchronizationSpec{APIVersion: apiVersion, Kind: kind, Mode: mode}
		c.Spec.Types = append(c.Spec.Types, spec)
		return updateHNCConfig(ctx, c)
	}).Should(Succeed())
}

// hasObject returns true if a namespace contains a specific object of the given kind.
//  The kind and its corresponding GVK should be included in the GVKs map.
func hasObject(ctx context.Context, kind string, nsName, name string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args.
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: name}
		inst := &unstructured.Unstructured{}
		inst.SetGroupVersionKind(GVKs[kind])
		err := k8sClient.Get(ctx, nnm, inst)
		return err == nil
	}
}

// makeObject creates an empty object of the given kind in a specific namespace. The kind and
// its corresponding GVK should be included in the GVKs map.
func makeObject(ctx context.Context, kind string, nsName, name string) {
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(GVKs[kind])
	inst.SetNamespace(nsName)
	inst.SetName(name)
	ExpectWithOffset(1, k8sClient.Create(ctx, inst)).Should(Succeed())
}

// objectInheritedFrom returns the name of the namespace where a specific object of a given kind
// is propagated from or an empty string if the object is not a propagated object. The kind and
// its corresponding GVK should be included in the GVKs map.
func objectInheritedFrom(ctx context.Context, kind string, nsName, name string) string {
	nnm := types.NamespacedName{Namespace: nsName, Name: name}
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(GVKs[kind])
	if err := k8sClient.Get(ctx, nnm, inst); err != nil {
		// should have been caught above
		return err.Error()
	}
	if inst.GetLabels() == nil {
		return ""
	}
	lif, _ := inst.GetLabels()["hnc.x-k8s.io/inheritedFrom"]
	return lif
}
