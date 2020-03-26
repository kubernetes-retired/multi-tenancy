package reconcilers_test

import (
	"context"
	"time"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

	It("should set the hns.status.state to Forbidden if the owner is not allowed to self-serve subnamespaces", func() {
		kube_system_hns_bar := newHierarchicalNamespace(barName, "kube-system")
		updateHierarchicalNamespace(ctx, kube_system_hns_bar)
		Eventually(getHNSState(ctx, "kube-system", barName)).Should(Equal(api.Forbidden))
	})

	It("should set the hns.status.state to Conflict if the namespace's owner annotation is wrong", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// It should set the self-serve subnamespace's owner annotation to the owner
		// namespace (should set bar's owner annotation to "foo").
		Eventually(func() string {
			return getNamespace(ctx, barName).GetAnnotations()[api.AnnotationOwner]
		}).Should(Equal(fooName))

		// Todo fix this flacky test on changing the namespace annotation. See issue:
		//	 https://github.com/kubernetes-sigs/multi-tenancy/issues/560
		// Sleep 1 second to avoid updating the namespace instance too quickly.
		time.Sleep(1 * time.Second)

		// Clear the owner annotation and the HNS state should be set to "Conflict".
		clearAnnotations(ctx, barName)
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Conflict))
	})

	It("should always set the owner as the parent if otherwise", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// Change the bar's parent. Additionally change another field to reflect the
		// update of the HC instance (hc.Spec.AllowCascadingDelete).
		Eventually(func() bool {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.AllowCascadingDelete
		}).Should(Equal(false))
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

func clearAnnotations(ctx context.Context, nnm string) {
	ns := &corev1.Namespace{}
	ns.Name = nnm
	updateNamespace(ctx, ns)
}

func updateNamespace(ctx context.Context, ns *corev1.Namespace) {
	EventuallyWithOffset(1, func() error {
		return k8sClient.Update(ctx, ns)
	}).Should(Succeed())
}
