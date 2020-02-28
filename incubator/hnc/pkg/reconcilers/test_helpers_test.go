package reconcilers_test

import (
	"context"
	"crypto/rand"
	"fmt"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
)

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

func makeRole(ctx context.Context, nsName, roleName string) {
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: nsName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		Rules: []v1.PolicyRule{
			// Allow the users to read all secrets, namespaces and configmaps.
			{
				APIGroups: []string{""},
				Resources: []string{"secrets", "namespaces", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, role)).Should(Succeed())
}

func hasRole(ctx context.Context, nsName, roleName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: roleName}
		role := &v1.Role{}
		err := k8sClient.Get(ctx, nnm, role)
		return err == nil
	}
}

func roleInheritedFrom(ctx context.Context, nsName, roleName string) string {
	nnm := types.NamespacedName{Namespace: nsName, Name: roleName}
	role := &v1.Role{}
	if err := k8sClient.Get(ctx, nnm, role); err != nil {
		// should have been caught above
		return err.Error()
	}
	if role.ObjectMeta.Labels == nil {
		return ""
	}
	lif, _ := role.ObjectMeta.Labels["hnc.x-k8s.io/inheritedFrom"]
	return lif
}
