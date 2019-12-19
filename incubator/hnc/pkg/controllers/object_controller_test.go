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
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
		bazName = createNS(ctx, "baz")

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

	It("should not be propagated if modified", func() {
		// Set tree as bar -> foo and make sure the first-time propagation of foo-sec
		// is finished before modifying the foo-sec in bar namespace
		setParent(ctx, barName, fooName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())

		// Wait 1 second to make sure all enqueued fooName hiers are successfully reconciled
		// in case the manual modification is overridden by the unfinished propagation.
		time.Sleep(1 * time.Second)
		modifySecret(ctx, barName, "foo-sec")

		// Set as parent. Give the controller a chance to copy the objects and make
		// sure that at least the correct one was copied. This gives us more confidence
		// that if the other one *isn't* copied, this is because we decided not to, and
		// not that we just haven't gotten to it yet.
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())

		// Make sure the bad one wasn't copied by the default(old) object controller
		// and got overwritten by the new object controller.
		if !newObjectController {
			Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
		} else {
			Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		}
	})

	It("should be removed if the source no longer exists", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())

		removeSecret(ctx, fooName, "foo-sec")
		// Wait 1 second to make sure the propagated objects are removed.
		time.Sleep(1 * time.Second)
		Eventually(hasSecret(ctx, fooName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
	})

	It("should overwrite the propagated ones if the source is updated", func() {
		if !newObjectController {
			return
		}
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(isModified(ctx, fooName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(isModified(ctx, barName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(isModified(ctx, bazName, "foo-sec")).Should(BeFalse())

		modifySecret(ctx, fooName, "foo-sec")
		// Wait 1 second to make sure the updated source get propagated.
		time.Sleep(1 * time.Second)
		Eventually(isModified(ctx, fooName, "foo-sec")).Should(BeTrue())
		Eventually(isModified(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(isModified(ctx, bazName, "foo-sec")).Should(BeTrue())
	})

	It("shouldn't propagate/delete if the namespace has Crit condition", func() {
		if !newObjectController {
			return
		}

		// Set tree as baz -> bar -> foo(root).
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)

		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))

		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "bar-sec")).Should(Equal(barName))

		// Set foo's parent to a non-existent namespace.
		brumpfName := createNSName("brumpf")
		fooHier := newOrGetHierarchy(ctx, fooName)
		fooHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritParentMissing)).Should(Equal(true))
		Eventually(hasCondition(ctx, barName, api.CritAncestor)).Should(Equal(true))
		Eventually(hasCondition(ctx, bazName, api.CritAncestor)).Should(Equal(true))

		// Set baz's parent to foo and add a new sec in foo.
		setParent(ctx, bazName, fooName)
		makeSecret(ctx, fooName, "foo-sec-2")

		// Wait 1 second to make sure any potential actions are done.
		time.Sleep(1 * time.Second)

		// Since the sync is frozen, baz should still have bar-sec (no deleting).
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "bar-sec")).Should(Equal(barName))
		// baz and bar shouldn't have foo-sec-2 (no propagating).
		Eventually(hasSecret(ctx, bazName, "foo-sec-2")).Should(BeFalse())
		Eventually(hasSecret(ctx, barName, "foo-sec-2")).Should(BeFalse())

		// Create the missing parent namespace with one object.
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())
		makeSecret(ctx, brumpfName, "brumpf-sec")

		// The Crit conditions should be gone.
		Eventually(hasCondition(ctx, fooName, api.CritParentMissing)).Should(Equal(false))
		Eventually(hasCondition(ctx, barName, api.CritAncestor)).Should(Equal(false))
		Eventually(hasCondition(ctx, bazName, api.CritAncestor)).Should(Equal(false))

		// Everything should be up to date after the Crit conditions are gone.
		Eventually(hasSecret(ctx, fooName, "brumpf-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, fooName, "brumpf-sec")).Should(Equal(brumpfName))

		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, barName, "foo-sec-2")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec-2")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, barName, "brumpf-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "brumpf-sec")).Should(Equal(brumpfName))

		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, bazName, "foo-sec-2")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec-2")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, bazName, "brumpf-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "brumpf-sec")).Should(Equal(brumpfName))

		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
	})

	It("should set conditions if it's excluded from being propagated, and clear them if it's fixed", func() {
		if !newObjectController {
			return
		}

		// Set tree as baz -> bar -> foo(root) and make sure the secret gets propagated.
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec")).Should(Equal(fooName))

		// Make the secret unpropagateable and verify that it disappears.
		setFinalizer(ctx, fooName, "foo-sec", true)
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())

		// Observe the condition on the source namespace
		want := &api.Condition{
			Code:    api.CannotPropagate,
			Affects: []api.AffectedObject{{Version: "v1", Kind: "Secret", Namespace: fooName, Name: "foo-sec"}},
		}
		Eventually(getCondition(ctx, fooName, api.CannotPropagate)).Should(Equal(want))

		// Fix the problem and verify that the condition vanishes and the secret is propagated again
		setFinalizer(ctx, fooName, "foo-sec", false)
		Eventually(hasCondition(ctx, fooName, api.CannotPropagate)).Should(Equal(false))
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, bazName, "foo-sec")).Should(Equal(fooName))
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

func setFinalizer(ctx context.Context, nsName, secretName string, set bool) {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, sec)).Should(Succeed())
	if set {
		sec.ObjectMeta.Finalizers = []string{"example.com/foo"}
	} else {
		sec.ObjectMeta.Finalizers = nil
	}
	ExpectWithOffset(1, k8sClient.Update(ctx, sec)).Should(Succeed())
}

func isModified(ctx context.Context, nsName, secretName string) bool {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, sec)).Should(Succeed())

	labels := sec.GetLabels()
	_, ok := labels["modify"]
	return ok
}

func removeSecret(ctx context.Context, nsName, secretName string) {
	sec := &corev1.Secret{}
	sec.Name = secretName
	sec.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Delete(ctx, sec)).Should(Succeed())
}
