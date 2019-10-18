package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("Secret", func() {
	ctx := context.Background()

	var (
		fooName  string
		barName  string
		bazName  string
		quxName  string
		quuxName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
		bazName = createNS(ctx, "baz")
		quxName = createNS(ctx, "qux")
		quuxName = createNS(ctx, "quux")

		// Give them each a secret
		makeSecret(ctx, fooName, "foo-sec")
		makeSecret(ctx, barName, "bar-sec")
		makeSecret(ctx, bazName, "baz-sec")
	})

	It("should be copied to descendents", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)

		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))

		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec")).Should(Equal(fooName))

		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "bar-sec")).Should(Equal(barName))
	})

	It("should be removed if the hierarchy changes", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		setParent(ctx, bazName, fooName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		setParent(ctx, bazName, "")
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
	})

	It("should set object conditions if not matching the source", func() {
		// Set tree as qux -> baz -> bar and make sure the propagation of bar-sec is
		// *fully finished* before modifying the bar-sec in baz namespace
		setParent(ctx, bazName, barName)
		setParent(ctx, quxName, bazName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quxName, "bar-sec")).Should(BeTrue())

		Eventually(hasAnyCondition(ctx, barName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, quxName)).Should(BeFalse())

		// Wait for 2 seconds for the object propagation from setParent() are finished.
		time.Sleep(2 * time.Second)
		modifySecret(ctx, quxName, "bar-sec")

		Eventually(hasCondition(ctx, quxName, api.ObjectOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, barName, api.ObjectDescendantOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, barName, api.ObjectOverridden)).Should(BeFalse())
		Eventually(hasCondition(ctx, quxName, api.ObjectDescendantOverridden)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())
	})

	It("should unset object conditions if it matches the source again", func() {
		// Set tree as qux -> baz -> bar and make sure the propagation of bar-sec is
		// *fully finished* before modifying the bar-sec in baz namespace
		setParent(ctx, bazName, barName)
		setParent(ctx, quxName, bazName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quxName, "bar-sec")).Should(BeTrue())

		Eventually(hasAnyCondition(ctx, barName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, quxName)).Should(BeFalse())

		time.Sleep(2 * time.Second)
		modifySecret(ctx, quxName, "bar-sec")

		Eventually(hasCondition(ctx, quxName, api.ObjectOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, barName, api.ObjectDescendantOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, barName, api.ObjectOverridden)).Should(BeFalse())
		Eventually(hasCondition(ctx, quxName, api.ObjectDescendantOverridden)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())

		// Restore the modified secret and make sure all object conditions are gone.
		time.Sleep(2 * time.Second)
		unmodifySecret(ctx, quxName, "bar-sec")

		Eventually(hasAnyCondition(ctx, quxName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, barName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())
	})

	It("should not be propagated if it has a modified ancestor(including itself)", func() {
		// Set tree as qux -> bar -> foo and make sure the propagation of foo-sec is
		// *fully finished* before modifying the foo-sec in qux namespace
		setParent(ctx, barName, fooName)
		setParent(ctx, quxName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quxName, "foo-sec")).Should(BeTrue())

		Eventually(hasAnyCondition(ctx, fooName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, barName)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, quxName)).Should(BeFalse())

		time.Sleep(2 * time.Second)
		modifySecret(ctx, quxName, "foo-sec")

		Eventually(hasCondition(ctx, quxName, api.ObjectOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, fooName, api.ObjectDescendantOverridden)).Should(BeTrue())
		Eventually(hasCondition(ctx, fooName, api.ObjectOverridden)).Should(BeFalse())
		Eventually(hasCondition(ctx, quxName, api.ObjectDescendantOverridden)).Should(BeFalse())
		Eventually(hasAnyCondition(ctx, barName)).Should(BeFalse())

		// Add baz to the hierarchy under qux, which has the modified secret, and make
		// sure the modified secret isn't copied but the unmodified one is. Give the
		// controller a chance to copy the objects and make sure that the correct one
		// was copied. This gives us more confidence that if the other one *isn't* copied,
		// this is because we decided not to, and not that we just haven't gotten to it yet.
		setParent(ctx, bazName, quxName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())

		// Make sure the bad one wasn't copied.
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())

		// Make sure no object conditions set in bazName
		Eventually(hasAnyCondition(ctx, bazName)).Should(BeFalse())

		// Make sure correct propagation on other clean branches
		setParent(ctx, quuxName, barName)
		Eventually(hasSecret(ctx, quuxName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quuxName, "bar-sec")).Should(BeTrue())
		Eventually(hasAnyCondition(ctx, quuxName)).Should(BeFalse())
	})

	It("should be removed if the source no longer exists", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())

		removeSecret(ctx, fooName, "foo-sec")
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
	})
})

func hasAnyCondition(ctx context.Context, nm string) func() bool {
	return func() bool {
		conds := getHierarchy(ctx, nm).Status.Conditions
		return conds != nil
	}
}

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

func secretInheritedFrom(ctx context.Context, nsName, secretName string) string {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	if err := k8sClient.Get(ctx, nnm, sec); err != nil {
		// should have been caught above
		return err.Error()
	}
	if sec.ObjectMeta.Labels == nil {
		return ""
	}
	lif, _ := sec.ObjectMeta.Labels["hnc.x-k8s.io/inheritedFrom"]
	return lif
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

func newOrGetHierarchy(ctx context.Context, nm string) *api.HierarchyConfiguration {
	hier := &api.HierarchyConfiguration{}
	hier.ObjectMeta.Namespace = nm
	hier.ObjectMeta.Name = api.Singleton
	snm := types.NamespacedName{Namespace: nm, Name: api.Singleton}
	if err := k8sClient.Get(ctx, snm, hier); err != nil {
		ExpectWithOffset(2, errors.IsNotFound(err)).Should(BeTrue())
	}
	return hier
}

func modifySecret(ctx context.Context, nsName, secretName string) {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, sec)).Should(Succeed())

	labels := sec.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["modify"] = "make-a-change"
	sec.SetLabels(labels)
	ExpectWithOffset(1, k8sClient.Update(ctx, sec)).Should(Succeed())
}

func unmodifySecret(ctx context.Context, nsName, secretName string) {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, sec)).Should(Succeed())

	labels := sec.GetLabels()
	delete(labels, "modify")
	sec.SetLabels(labels)
	ExpectWithOffset(1, k8sClient.Update(ctx, sec)).Should(Succeed())
}

func removeSecret(ctx context.Context, nsName, secretName string) {
	sec := &corev1.Secret{}
	sec.Name = secretName
	sec.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Delete(ctx, sec)).Should(Succeed())
}
