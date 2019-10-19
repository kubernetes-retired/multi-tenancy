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
		fooName string
		barName string
		bazName string
		quxName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
		bazName = createNS(ctx, "baz")
		quxName = createNS(ctx, "qux")

		// Give them each a secret
		makeSecret(ctx, fooName, "foo-sec")
		makeSecret(ctx, barName, "bar-sec")
		makeSecret(ctx, bazName, "baz-sec")
	})

	It("should be copied to descendents", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
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

	It("should be marked as modified if not matching the source", func() {
		// Set tree as qux -> baz -> bar and make sure the propagation of bar-sec is
		// *fully finished* before modifying the bar-sec in baz namespace
		setParent(ctx, bazName, barName)
		setParent(ctx, quxName, bazName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quxName, "bar-sec")).Should(BeTrue())

		Eventually(hasAnnotationModified(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasAnnotationModified(ctx, bazName, "baz-sec")).Should(BeFalse())

		modifySecret(ctx, bazName, "bar-sec")

		// Wait 5s for the annotation to be updated. Even 4s may have timeout error.
		// TODO: revisit this to see why it's so slow.
		Eventually(hasAnnotationModified(ctx, bazName, "bar-sec"), 5*time.Second).Should(BeTrue())
		Eventually(hasAnnotationModified(ctx, bazName, "baz-sec")).Should(BeFalse())
	})

	It("should not be propagated if marked as modified", func() {
		// Set tree as qux -> bar -> foo and make sure the propagation of foo-sec is
		// *fully finished* before modifying the foo-sec in bar namespace
		setParent(ctx, barName, fooName)
		setParent(ctx, quxName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, quxName, "foo-sec")).Should(BeTrue())

		modifySecret(ctx, barName, "foo-sec")

		// Wait 5s for the annotation to be updated. Even 4s may have timeout error.
		// TODO: revisit this. There may be racing updates between modifySecret and setParent.
		Eventually(hasAnnotationModified(ctx, barName, "foo-sec"), 5*time.Second).Should(BeTrue())

		// Change the parent. Give the controller a chance to copy the objects and make
		// sure that at least the correct one was copied. This gives us more confidence
		// that if the other one *isn't* copied, this is because we decided not to, and
		// not that we just haven't gotten to it yet.
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())

		// Make sure the bad one wasn't copied.
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
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

func hasAnnotationModified(ctx context.Context, nsName, secretName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
		sec := &corev1.Secret{}
		ExpectWithOffset(1, k8sClient.Get(ctx, nnm, sec)).Should(Succeed())
		return sec.GetAnnotations()["hnc.x-k8s.io/modified"] == "true"
	}
}

func removeSecret(ctx context.Context, nsName, secretName string) {
	sec := &corev1.Secret{}
	sec.Name = secretName
	sec.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Delete(ctx, sec)).Should(Succeed())
}
