package controllers_test

import (
	"context"
	"time"

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

	It("should set the parent to a bad value if the parent is deleted", func() {
		// Set up the parent-child relationship
		barHier := getHierarchy(ctx, barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		Eventually(func() []string {
			defHier := getHierarchy(ctx, fooName)
			return defHier.Status.Children
		}).Should(Equal([]string{barName}))

		// Delete the parent. We can't actually delete the namespace because the test env
		// doesn't run the builting Namespace controller, but we *can* mark it as deleted
		// and also delete the singleton, which should be enough to prevent it from being
		// recreated as well as force the reconciler to believe that the namespace is gone.
		fooHier := getHierarchy(ctx, fooName)
		fooNS := &corev1.Namespace{}
		fooNS.Name = fooName
		Expect(k8sClient.Delete(ctx, fooNS)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, fooHier)).Should(Succeed())

		// Verify that the parent pointer is now set to a bad value
		Eventually(func() string {
			barHier = getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal("missing parent " + fooName))
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
