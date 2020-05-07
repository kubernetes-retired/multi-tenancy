package reconcilers_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("HierarchicalNamespace", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNSName("bar")
	})

	It("should create an owned namespace and update the hierarchy according to the HNS instance", func() {
		// Create 'bar' hns in 'foo' namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// It should create the namespace 'bar' with 'foo' in the owner annotation.
		Eventually(func() string {
			return getNamespace(ctx, barName).GetAnnotations()[api.AnnotationOwner]
		}).Should(Equal(fooName))

		// It should set the owned namespace "bar" as a child of the owner "foo".
		Eventually(func() []string {
			fooHier := getHierarchy(ctx, fooName)
			return fooHier.Status.Children
		}).Should(Equal([]string{barName}))

		// It should set the owner namespace "foo" as the parent of "bar".
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))

		// It should set the hns.status.state to Ok if the above sub-tests all pass.
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Ok))
	})

	It("should set the hns.status.state to Forbidden if the owner is not allowed to subnamespaces", func() {
		kube_system_hns_bar := newHierarchicalNamespace(barName, "kube-system")
		updateHierarchicalNamespace(ctx, kube_system_hns_bar)
		Eventually(getHNSState(ctx, "kube-system", barName)).Should(Equal(api.Forbidden))
	})

	It("should set the hns.status.state to Conflict if a namespace of the same name already exists", func() {
		// Create "baz" namespace.
		bazName := createNS(ctx, "baz")
		// Create an hns instance still with the same name "baz" in "foo" namespace.
		foo_hns_baz := newHierarchicalNamespace(bazName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_baz)
		Eventually(getHNSState(ctx, fooName, bazName)).Should(Equal(api.Conflict))
	})

	It("should always set the owner as the parent if otherwise", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))

		// Change the bar's parent. Additionally change another field to reflect the
		// update of the HC instance (hc.Spec.AllowCascadingDelete).
		barHier := getHierarchy(ctx, barName)
		barHier.Spec.Parent = "other"
		barHier.Spec.AllowCascadingDelete = true
		updateHierarchy(ctx, barHier)

		// The parent of 'bar' should be set back to 'foo' after reconciliation.
		Eventually(func() bool {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.AllowCascadingDelete
		}).Should(Equal(true))
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))
	})
})

func getHNSState(ctx context.Context, pnm, nm string) func() api.HNSState {
	return func() api.HNSState {
		return getHierarchicalNamespace(ctx, pnm, nm).Status.State
	}
}

func newHierarchicalNamespace(hnsnm, nm string) *api.HierarchicalNamespace {
	hns := &api.HierarchicalNamespace{}
	hns.ObjectMeta.Namespace = nm
	hns.ObjectMeta.Name = hnsnm
	return hns
}

func getHierarchicalNamespace(ctx context.Context, pnm, nm string) *api.HierarchicalNamespace {
	return getHierarchicalNamespaceWithOffset(1, ctx, pnm, nm)
}

func getHierarchicalNamespaceWithOffset(offset int, ctx context.Context, pnm, nm string) *api.HierarchicalNamespace {
	nsn := types.NamespacedName{Name: nm, Namespace: pnm}
	hns := &api.HierarchicalNamespace{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, nsn, hns)
	}).Should(Succeed())
	return hns
}

func updateHierarchicalNamespace(ctx context.Context, hns *api.HierarchicalNamespace) {
	if hns.CreationTimestamp.IsZero() {
		ExpectWithOffset(1, k8sClient.Create(ctx, hns)).Should(Succeed())
	} else {
		ExpectWithOffset(1, k8sClient.Update(ctx, hns)).Should(Succeed())
	}
}
