package controllers_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("Secret", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
		bazName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
		bazName = createNS(ctx, "baz")

		// Start with the tree foo -> bar -> baz
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)

		// Give them each a secret
		makeSecret(ctx, fooName, "foo-sec")
		makeSecret(ctx, barName, "bar-sec")
		makeSecret(ctx, bazName, "baz-sec")
	})

	It("should be copied to descendents", func() {
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
	})

	It("should be removed if the hierarchy changes", func() {
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		setParent(ctx, bazName, fooName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		setParent(ctx, bazName, "")
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
	})
})

func makeSecret(ctx context.Context, nsName, secretName string) {
	sec := &corev1.Secret{}
	sec.Name = secretName
	sec.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Create(ctx, sec)).Should(Succeed())
}

func hasSecret(ctx context.Context, nsName, secretName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
		sec := &corev1.Secret{}
		err := k8sClient.Get(ctx, nnm, sec)
		return err == nil
	}
}

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

func newOrGetHierarchy(ctx context.Context, nm string) *tenancy.Hierarchy {
	hier := &tenancy.Hierarchy{}
	hier.ObjectMeta.Namespace = nm
	hier.ObjectMeta.Name = tenancy.Singleton
	snm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	if err := k8sClient.Get(ctx, snm, hier); err != nil {
		ExpectWithOffset(2, errors.IsNotFound(err)).Should(BeTrue())
	}
	return hier
}
