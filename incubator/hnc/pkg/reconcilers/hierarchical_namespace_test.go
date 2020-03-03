package reconcilers_test

import (
	"context"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Hierarchy", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		if !enableHNSReconciler {
			Skip("Skipping hierarchical namespace tests when the hns reconciler is not enabled.")
		}

		fooName = createNS(ctx, "foo")
		barName = createNSName("bar")

		// Create "bar" hns in "foo" namespace
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)
	})

	It("should set the self-serve subnamespace as a child on the current namespace", func() {
		Eventually(func() []string {
			fooHier := getHierarchy(ctx, fooName)
			return fooHier.Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set the current namespace as the parent of the self-serve subnamespace", func() {
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))
	})

	It("should create the self-serve subnamespace", func() {
		nnm := types.NamespacedName{Name: barName}
		ns := &corev1.Namespace{}
		Eventually(func() error {
			return k8sClient.Get(ctx, nnm, ns)
		}).Should(Succeed())
	})

	It("should set the self-serve subnamespace's owner annotation to the current namespace", func() {
		Eventually(getNamespaceAnnotation(ctx, barName, api.AnnotationOwner)).Should(Equal(fooName))
	})
})

func newHierarchicalNamespace(hnsnm, nm string) *api.HierarchicalNamespace {
	hns := &api.HierarchicalNamespace{}
	hns.ObjectMeta.Namespace = nm
	hns.ObjectMeta.Name = hnsnm
	return hns
}

func updateHierarchicalNamespace(ctx context.Context, hns *api.HierarchicalNamespace) {
	if hns.CreationTimestamp.IsZero() {
		ExpectWithOffset(1, k8sClient.Create(ctx, hns)).Should(Succeed())
	} else {
		ExpectWithOffset(1, k8sClient.Update(ctx, hns)).Should(Succeed())
	}
}

func getNamespaceAnnotation(ctx context.Context, nnm, annotation string) func() string {
	return func() string {
		ns := getNamespace(ctx, nnm)
		val, _ := ns.GetAnnotations()[annotation]
		return val
	}
}
