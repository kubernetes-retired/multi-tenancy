package reconcilers_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("Anchor", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNSName("bar")
	})

	It("should create an subnamespace and update the hierarchy according to the anchor", func() {
		// Create 'bar' anchor in 'foo' namespace.
		foo_anchor_bar := newAnchor(barName, fooName)
		updateAnchor(ctx, foo_anchor_bar)

		// It should create the namespace 'bar' with 'foo' in the subnamespaceOf annotation.
		Eventually(func() string {
			return getNamespace(ctx, barName).GetAnnotations()[api.SubnamespaceOf]
		}).Should(Equal(fooName))

		// It should set the subnamespace "bar" as a child of the parent "foo".
		Eventually(func() []string {
			fooHier := getHierarchy(ctx, fooName)
			return fooHier.Status.Children
		}).Should(Equal([]string{barName}))

		// It should set the parent namespace "foo" as the parent of "bar".
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))

		// It should set the anchor.status.state to Ok if the above sub-tests all pass.
		Eventually(getAnchorState(ctx, fooName, barName)).Should(Equal(api.Ok))
	})

	It("should set the anchor.status.state to Forbidden if the parent is not allowed to have subnamespaces", func() {
		kube_system_anchor_bar := newAnchor(barName, "kube-system")
		updateAnchor(ctx, kube_system_anchor_bar)
		Eventually(getAnchorState(ctx, "kube-system", barName)).Should(Equal(api.Forbidden))
	})

	It("should set the anchor.status.state to Conflict if a namespace of the same name already exists", func() {
		// Create "baz" namespace.
		bazName := createNS(ctx, "baz")
		// Create an anchor still with the same name "baz" in "foo" namespace.
		foo_anchor_baz := newAnchor(bazName, fooName)
		updateAnchor(ctx, foo_anchor_baz)
		Eventually(getAnchorState(ctx, fooName, bazName)).Should(Equal(api.Conflict))
	})

	It("should always set the owner as the parent if otherwise", func() {
		// Create "bar" anchor in "foo" namespace.
		foo_anchor_bar := newAnchor(barName, fooName)
		updateAnchor(ctx, foo_anchor_bar)
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

func getAnchorState(ctx context.Context, pnm, nm string) func() api.SubnamespaceAnchorState {
	return func() api.SubnamespaceAnchorState {
		return getAnchor(ctx, pnm, nm).Status.State
	}
}

func newAnchor(anm, nm string) *api.SubnamespaceAnchor {
	anchor := &api.SubnamespaceAnchor{}
	anchor.ObjectMeta.Namespace = nm
	anchor.ObjectMeta.Name = anm
	return anchor
}

func getAnchor(ctx context.Context, pnm, nm string) *api.SubnamespaceAnchor {
	return getAnchorWithOffset(1, ctx, pnm, nm)
}

func getAnchorWithOffset(offset int, ctx context.Context, pnm, nm string) *api.SubnamespaceAnchor {
	nsn := types.NamespacedName{Name: nm, Namespace: pnm}
	anchor := &api.SubnamespaceAnchor{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, nsn, anchor)
	}).Should(Succeed())
	return anchor
}

func updateAnchor(ctx context.Context, anchor *api.SubnamespaceAnchor) {
	if anchor.CreationTimestamp.IsZero() {
		ExpectWithOffset(1, k8sClient.Create(ctx, anchor)).Should(Succeed())
	} else {
		ExpectWithOffset(1, k8sClient.Update(ctx, anchor)).Should(Succeed())
	}
}
