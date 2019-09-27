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

	It("should set a condition if the parent is missing", func() {
		// Set up the parent-child relationship
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = "brumpf"
		updateHierarchy(ctx, barHier)
		Eventually(func() bool {
			barHier = getHierarchy(ctx, barName)
			for _, cond := range barHier.Status.Conditions {
				if cond.Msg == "missing parent" {
					return true
				}
			}
			return false
		}).Should(Equal(true))
	})

	It("should resolve the condition if the parent is later created", func() {
		// Set up the parent-child relationship with the missing name
		brumpfName := createNSName("brumpf")
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, barHier)
		isMissing := func() bool {
			barHier = getHierarchy(ctx, barName)
			for _, cond := range barHier.Status.Conditions {
				if cond.Msg == "missing parent" {
					return true
				}
			}
			return false
		}
		Eventually(isMissing).Should(Equal(true))

		// Create the missing parent
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())

		// Ensure the condition is resolved on the child
		Eventually(isMissing).Should(Equal(false))

		// Ensure the child is listed on the parent
		Eventually(func() []string {
			brumpfHier := getHierarchy(ctx, brumpfName)
			return brumpfHier.Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set a condition if a self-cycle is detected", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = fooName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []tenancy.Condition {
			return getHierarchy(ctx, fooName).Status.Conditions
		}).ShouldNot(BeNil())
	})

	It("should set a condition if a cycle is detected", func() {
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
		Eventually(func() []tenancy.Condition {
			return getHierarchy(ctx, fooName).Status.Conditions
		}).ShouldNot(BeNil())
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

	It("should report a condition if a required child belongs elsewhere", func() {
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

		// Verify that all three namespaces have at least one condition
		Eventually(func() bool {
			conds := getHierarchy(ctx, fooName).Status.Conditions
			if len(conds) > 0 {
				fmt.Printf("Conditions for foo: %+v\n", conds)
			}
			return len(conds) > 0
		}).Should(BeTrue())
		Eventually(func() bool {
			conds := getHierarchy(ctx, barName).Status.Conditions
			if len(conds) > 0 {
				fmt.Printf("Conditions for bar: %+v\n", conds)
			}
			return len(conds) > 0
		}).Should(BeTrue())
		Eventually(func() bool {
			conds := getHierarchy(ctx, bazName).Status.Conditions
			if len(conds) > 0 {
				fmt.Printf("Conditions for baz: %+v\n", conds)
			}
			return len(conds) > 0
		}).Should(BeTrue())
	})
})

func newHierarchy(nm string) *tenancy.Hierarchy {
	hier := &tenancy.Hierarchy{}
	hier.ObjectMeta.Namespace = nm
	hier.ObjectMeta.Name = tenancy.Singleton
	return hier
}

func getHierarchy(ctx context.Context, nm string) *tenancy.Hierarchy {
	return getHierarchyWithOffset(1, ctx, nm)
}

func getHierarchyWithOffset(offset int, ctx context.Context, nm string) *tenancy.Hierarchy {
	snm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	hier := &tenancy.Hierarchy{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, snm, hier)
	}).Should(Succeed())
	return hier
}

func updateHierarchy(ctx context.Context, h *tenancy.Hierarchy) {
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
