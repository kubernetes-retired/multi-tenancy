package reconcilers_test

import (
	"context"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/reconcilers"
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
		if !enableHNSReconciler {
			Skip("Skipping hierarchical namespace tests when the hns reconciler is not enabled.")
		}

		fooName = createNS(ctx, "foo")
		barName = createNSName("bar")
	})

	It("should create the subnamespace and update the hierarchy according to the HNS instance", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// It should set the self-serve subnamespace "bar" as a child on the owner
		// namespace "foo".
		Eventually(func() []string {
			fooHier := getHierarchy(ctx, fooName)
			return fooHier.Status.Children
		}).Should(Equal([]string{barName}))

		// It should set the owner namespace "foo" as the parent of the self-serve
		// subnamespace "bar".
		Eventually(func() string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Spec.Parent
		}).Should(Equal(fooName))

		// It should create the self-serve subnamespace "bar".
		nnm := types.NamespacedName{Name: barName}
		ns := &corev1.Namespace{}
		Eventually(func() error {
			return k8sClient.Get(ctx, nnm, ns)
		}).Should(Succeed())

		// It should set the self-serve subnamespace's owner annotation to the owner
		// namespace (should set bar's owner annotation to "foo").
		Eventually(getNamespaceAnnotation(ctx, barName, api.AnnotationOwner)).Should(Equal(fooName))

		// It should set the hns.status.state to Ok if the above sub-tests all pass.
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Ok))
	})

	It("should set the hns.status.state to Forbidden if the parent namespace is not allowed to self-serve subnamespaces", func() {
		kube_system_hns_bar := newHierarchicalNamespace(barName, "kube-system")
		updateHierarchicalNamespace(ctx, kube_system_hns_bar)
		Eventually(getHNSState(ctx, "kube-system", barName)).Should(Equal(api.Forbidden))
	})

	It("should set the hns.status.state to Missing if the self-serve subnamespace doesn't exist", func() {
		// This is a trick to disable hc reconciler on "bar" namespace by having "bar" in the excluded namespace list.
		// Therefore the "bar" namespace won't be created even if the HNS reconciler enqueues the not-yet existing
		// "bar" namespace for hc reconciler to reconcile and create.
		reconcilers.EX[barName] = true

		// Create "bar" hns in "foo" namespace after the HC reconciler is "disabled" (only for "bar" namespace).
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// We should then have the HNS state stays at "Missing"
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Missing))
	})

	It("should set the hns.status.state to Conflict if the namespace's owner annotation is wrong", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// It should set the self-serve subnamespace's owner annotation to the owner
		// namespace (should set bar's owner annotation to "foo").
		Eventually(getNamespaceAnnotation(ctx, barName, api.AnnotationOwner)).Should(Equal(fooName))

		// Change the owner annotation to a different value and the HNS state should
		// be set to "Conflict".
		setWrongNamespaceOwnerAnnotation(ctx, barName)
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Conflict))
	})

	It("should set the hns.status.state to Conflict if the namespace's parent is not the owner", func() {
		// Create "bar" hns in "foo" namespace.
		foo_hns_bar := newHierarchicalNamespace(barName, fooName)
		updateHierarchicalNamespace(ctx, foo_hns_bar)

		// Change the bar's parent. The HNS should be set to "Conflict".
		barHier := getHierarchy(ctx, barName)
		barHier.Spec.Parent = "other"
		updateHierarchy(ctx, barHier)
		Eventually(getHNSState(ctx, fooName, barName)).Should(Equal(api.Conflict))
	})
})

func getHNSState(ctx context.Context, pnm, nm string) func() api.HNSState {
	return func() api.HNSState {
		return getHierarchicalNamespace(ctx, pnm, nm).Status.State
	}
}

func getNamespaceAnnotation(ctx context.Context, nnm, annotation string) func() string {
	return func() string {
		ns := getNamespace(ctx, nnm)
		val, _ := ns.GetAnnotations()[annotation]
		return val
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

func setWrongNamespaceOwnerAnnotation(ctx context.Context, nnm string) {
	a := make(map[string]string)
	a[api.AnnotationOwner] = "wrong"
	ns := getNamespace(ctx, nnm)
	ns.SetAnnotations(a)
	updateNamespace(ctx, nnm)
}

func updateNamespace(ctx context.Context, nm string) {
	ns := &corev1.Namespace{}
	ns.ObjectMeta.Name = nm
	EventuallyWithOffset(1, func() error {
		return k8sClient.Update(ctx, ns)
	}).Should(Succeed())
}
