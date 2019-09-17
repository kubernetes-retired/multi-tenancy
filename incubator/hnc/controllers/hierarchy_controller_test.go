package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

		// Add a quick delay so that the controller is idle
		// TODO: look into why the controller is missing events.
		time.Sleep(100 * time.Millisecond)
	})

	It("should set a child on the parent", func() {
		fooHier := getHierarchy(ctx, fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Status.Children
		}).Should(Equal([]string{fooName}))
	})

	It("should set a condition if the parent is missing", func() {
		// Set up the parent-child relationship
		barHier := getHierarchy(ctx, barName)
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

	It("should set a condition if a self-cycle is detected", func() {
		fooHier := getHierarchy(ctx, fooName)
		fooHier.Spec.Parent = fooName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []tenancy.Condition {
			return getHierarchy(ctx, fooName).Status.Conditions
		}).ShouldNot(BeNil())
	})

	It("should set a condition if a cycle is detected", func() {
		// Set up initial hierarchy
		barHier := getHierarchy(ctx, barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))

		// Wait for the controller to become idle
		time.Sleep(0 * time.Second)

		// Break it
		fooHier := getHierarchy(ctx, fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []tenancy.Condition {
			return getHierarchy(ctx, fooName).Status.Conditions
		}).ShouldNot(BeNil())
	})
})

func getHierarchy(ctx context.Context, nm string) *tenancy.Hierarchy {
	return getHierarchyWithOffset(1, ctx, nm)
}

func getHierarchyWithOffset(offset int, ctx context.Context, nm string) *tenancy.Hierarchy {
	snm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	hier := &tenancy.Hierarchy{}
	ExpectWithOffset(offset+1, k8sClient.Get(ctx, snm, hier)).Should(Succeed())
	return hier
}

func updateHierarchy(ctx context.Context, h *tenancy.Hierarchy) {
	Expect(k8sClient.Update(ctx, h)).Should(Succeed())
}
