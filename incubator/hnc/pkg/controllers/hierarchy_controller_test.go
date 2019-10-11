package controllers_test

import (
	"context"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

const (
	metaGroup = "hnc.x-k8s.io"
)

var _ = Describe("Hierarchy", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
	})

	It("should set a child on the parent", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Status.Children
		}).Should(Equal([]string{fooName}))
	})

	It("should set CritParentMissing condition if the parent is missing", func() {
		// Set up the parent-child relationship
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = "brumpf"
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(true))
	})

	It("should unset CritParentMissing condition if the parent is later created", func() {
		// Set up the parent-child relationship with the missing name
		brumpfName := createNSName("brumpf")
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(true))

		// Create the missing parent
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())

		// Ensure the condition is resolved on the child
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(false))

		// Ensure the child is listed on the parent
		Eventually(func() []string {
			brumpfHier := getHierarchy(ctx, brumpfName)
			return brumpfHier.Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set CritAncestor condition if any ancestor has critical condition", func() {
		// Set up the parent-child relationship
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = "brumpf"
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(true))

		// Set bar as foo's parent
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, tenancy.CritAncestor)).Should(Equal(true))
	})

	It("should unset CritAncestor condition if critical conditions in ancestors are gone", func() {
		// Set up the parent-child relationship with the missing name
		brumpfName := createNSName("brumpf")
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(true))

		// Set bar as foo's parent
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, tenancy.CritAncestor)).Should(Equal(true))

		// Create the missing parent
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())

		// Ensure the condition is resolved on the child
		Eventually(hasCondition(ctx, barName, tenancy.CritParentMissing)).Should(Equal(false))

		// Ensure the child is listed on the parent
		Eventually(func() []string {
			brumpfHier := getHierarchy(ctx, brumpfName)
			return brumpfHier.Status.Children
		}).Should(Equal([]string{barName}))

		// Ensure foo is enqueued and thus get CritAncestor condition updated after
		// critical conditions are resolved in bar.
		Eventually(hasCondition(ctx, fooName, tenancy.CritAncestor)).Should(Equal(false))
	})

	It("should set CritParentInvalid condition if a self-cycle is detected", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = fooName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, tenancy.CritParentInvalid)).Should(Equal(true))
	})

	It("should set CritParentInvalid condition if a cycle is detected", func() {
		// Set up initial hierarchy
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))

		// Break it
		fooHier := getHierarchy(ctx, fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, tenancy.CritParentInvalid)).Should(Equal(true))
	})

	It("should create a child namespace if requested", func() {
		// Create a namespace with a required child
		fooHier := newHierarchy(fooName)
		fooHier.Spec.RequiredChildren = []string{barName}
		updateHierarchy(ctx, fooHier)

		// Verify that it exists
		Eventually(func() string {
			return getHierarchy(ctx, barName).Spec.Parent
		}).Should(Equal(fooName))
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set CritRequiredChildConflict condition if a required child belongs elsewhere", func() {
		bazName := createNS(ctx, "baz")

		// Make baz a child of foo
		bazHier := newHierarchy(bazName)
		bazHier.Spec.Parent = fooName
		updateHierarchy(ctx, bazHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{bazName}))

		// Try to also make baz a required child of bar
		barHier := newHierarchy(barName)
		barHier.Spec.RequiredChildren = []string{bazName}
		updateHierarchy(ctx, barHier)

		// Verify that all three namespaces have CritRequiredChildConflict condition set.
		Eventually(hasCondition(ctx, fooName, tenancy.CritRequiredChildConflict)).Should(Equal(true))
		Eventually(hasCondition(ctx, barName, tenancy.CritRequiredChildConflict)).Should(Equal(true))
		Eventually(hasCondition(ctx, bazName, tenancy.CritRequiredChildConflict)).Should(Equal(true))
	})

	It("should have a tree label", func() {
		// Make bar a child of foo
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		// First, verify bar is a child of foo
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))
		// Verify that bar has a tree label related to foo
		Eventually(func() bool {
			barNS := getNamespace(ctx, barName)
			_, ok := barNS.GetLabels()[fooName+".tree."+metaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			barNS := getNamespace(ctx, barName)
			val, _ := barNS.GetLabels()[fooName+".tree."+metaGroup+"/depth"]
			return val
		}).Should(Equal("1"))
		// Verify that bar has a tree label related to bar itself
		Eventually(func() bool {
			barNS := getNamespace(ctx, barName)
			_, ok := barNS.GetLabels()[barName+".tree."+metaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			barNS := getNamespace(ctx, barName)
			val, _ := barNS.GetLabels()[barName+".tree."+metaGroup+"/depth"]
			return val
		}).Should(Equal("0"))
		// Verify that foo has a tree label related to foo itself
		Eventually(func() bool {
			fmt.Println(getHierarchy(ctx, fooName))
			fooNS := getNamespace(ctx, fooName)
			_, ok := fooNS.GetLabels()[fooName+".tree."+metaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			fooNS := getNamespace(ctx, fooName)
			val, _ := fooNS.GetLabels()[fooName+".tree."+metaGroup+"/depth"]
			return val
		}).Should(Equal("0"))
	})
})

func hasCondition(ctx context.Context, nm string, code tenancy.Code) func() bool {
	return func() bool {
		conds := getHierarchy(ctx, nm).Status.Conditions
		for _, cond := range conds {
			if cond.Code == code {
				return true
			}
		}
		return false
	}
}

func newHierarchy(nm string) *tenancy.HierarchyConfiguration {
	hier := &tenancy.HierarchyConfiguration{}
	hier.ObjectMeta.Namespace = nm
	hier.ObjectMeta.Name = tenancy.Singleton
	return hier
}

func getHierarchy(ctx context.Context, nm string) *tenancy.HierarchyConfiguration {
	return getHierarchyWithOffset(1, ctx, nm)
}

func getHierarchyWithOffset(offset int, ctx context.Context, nm string) *tenancy.HierarchyConfiguration {
	snm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	hier := &tenancy.HierarchyConfiguration{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, snm, hier)
	}).Should(Succeed())
	return hier
}

func getNamespace(ctx context.Context, nm string) *corev1.Namespace {
	return getNamespaceWithOffset(1, ctx, nm)
}

func getNamespaceWithOffset(offset int, ctx context.Context, nm string) *corev1.Namespace {
	nnm := types.NamespacedName{Name: nm}
	ns := &corev1.Namespace{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, nnm, ns)
	}).Should(Succeed())
	return ns
}

func updateHierarchy(ctx context.Context, h *tenancy.HierarchyConfiguration) {
	if h.CreationTimestamp.IsZero() {
		Expect(k8sClient.Create(ctx, h)).Should(Succeed())
	} else {
		Expect(k8sClient.Update(ctx, h)).Should(Succeed())
	}
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
