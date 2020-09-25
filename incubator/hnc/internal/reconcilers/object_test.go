package reconcilers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
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

		// We want to ensure we're working with a clean slate, in case a previous tests objects still exist
		cleanupObjects(ctx)

		// Give them each a role.
		makeObject(ctx, "Role", fooName, "foo-role")
		makeObject(ctx, "Role", barName, "bar-role")
		makeObject(ctx, "Role", bazName, "baz-role")
	})

	AfterEach(func() {
		// Change current config back to the default value.
		Eventually(func() error {
			return resetHNCConfigToDefault(ctx)
		}).Should(Succeed())
		cleanupObjects(ctx)
	})

	// Exceptions have not been implemented, so we are ignoring the following four tests for now.
	// They should be enabled once exception is done.
	PIt("should propagate object only to selected namespace using treeSelect", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, fooName)

		// Create a Role that does NOT propogate to bazName
		a := map[string]string{"propagate.hnc.x-k8s.io/treeSelect": "!" + bazName}
		makeObjectWithAnnotation(ctx, "Role", fooName, "testrole", a)

		Eventually(hasObject(ctx, "Role", barName, "testrole")).Should(BeTrue())
		Consistently(hasObject(ctx, "Role", bazName, "testrole")).Should(BeFalse())
	})

	It("should propagate object only to selected namespace using select", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, fooName)

		// Create a Role that does NOT propogate to bazName
		a := map[string]string{"propagate.hnc.x-k8s.io/select": "!" + bazName + ".tree.hnc.x-k8s.io/depth"}
		makeObjectWithAnnotation(ctx, "Role", fooName, "testrole", a)

		Eventually(hasObject(ctx, "Role", barName, "testrole")).Should(BeTrue())
		Consistently(hasObject(ctx, "Role", bazName, "testrole")).Should(BeFalse())
	})

	It("should remove object in selected namespace when selector is updated", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, fooName)

		makeObject(ctx, "Role", fooName, "testrole")
		Eventually(hasObject(ctx, "Role", barName, "testrole")).Should(BeTrue())
		Eventually(hasObject(ctx, "Role", bazName, "testrole")).Should(BeTrue())

		// Update the Role so that it does NOT propogate to bazName
		a := map[string]string{"propagate.hnc.x-k8s.io/select": "!" + bazName + ".tree.hnc.x-k8s.io/depth"}
		Expect(updateObjectWithAnnotation(ctx, "Role", fooName, "testrole", a)).Should(Succeed())

		Consistently(hasObject(ctx, "Role", barName, "testrole")).Should(BeTrue())
		Eventually(hasObject(ctx, "Role", bazName, "testrole")).Should(BeFalse())
	})

	PIt("should not propagate object to any namespace using none", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, fooName)

		// Create a Role that does NOT propogate to bazName
		a := map[string]string{"propagate.hnc.x-k8s.io/none": "true"}
		makeObjectWithAnnotation(ctx, "Role", fooName, "testrole", a)

		Consistently(hasObject(ctx, "Role", barName, "testrole")).Should(BeFalse())
		Consistently(hasObject(ctx, "Role", bazName, "testrole")).Should(BeFalse())
	})

	It("should be copied to descendents", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))

		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role")).Should(Equal(fooName))

		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "bar-role")).Should(Equal(barName))
	})

	It("should be copied to descendents when source object is empty", func() {
		setParent(ctx, barName, fooName)
		// Creates an empty ConfigMap. We use ConfigMap for this test because the apiserver will not
		// add additional fields to an empty ConfigMap object to make it non-empty.
		makeObject(ctx, "ConfigMap", fooName, "foo-config")
		addToHNCConfig(ctx, "v1", "ConfigMap", api.Propagate)

		// "foo-config" should now be propagated from foo to bar.
		Eventually(hasObject(ctx, "ConfigMap", barName, "foo-config")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "ConfigMap", barName, "foo-config")).Should(Equal(fooName))
	})

	It("should be removed if the hierarchy changes", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		setParent(ctx, bazName, fooName)
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		setParent(ctx, bazName, "")
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeFalse())
	})

	It("should not be propagated if modified", func() {
		// Set tree as bar -> foo and make sure the first-time propagation of foo-role
		// is finished before modifying the foo-role in bar namespace
		setParent(ctx, barName, fooName)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())

		// Wait 1 second to make sure all enqueued fooName hiers are successfully reconciled
		// in case the manual modification is overridden by the unfinished propagation.
		time.Sleep(1 * time.Second)
		modifyRole(ctx, barName, "foo-role")

		// Set as parent. Give the reconciler a chance to copy the objects and make
		// sure that at least the correct one was copied. This gives us more confidence
		// that if the other one *isn't* copied, this is because we decided not to, and
		// not that we just haven't gotten to it yet.
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())

		// Make sure the bad one got overwritte.
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
	})

	It("should be removed if the source no longer exists", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())

		removeRole(ctx, fooName, "foo-role")
		Eventually(hasObject(ctx, "Role", fooName, "foo-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeFalse())
	})

	It("should overwrite the propagated ones if the source is updated", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", fooName, "foo-role")).Should(BeTrue())
		Eventually(isModified(ctx, fooName, "foo-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Eventually(isModified(ctx, barName, "foo-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Eventually(isModified(ctx, bazName, "foo-role")).Should(BeFalse())

		modifyRole(ctx, fooName, "foo-role")
		Eventually(isModified(ctx, fooName, "foo-role")).Should(BeTrue())
		Eventually(isModified(ctx, barName, "foo-role")).Should(BeTrue())
		Eventually(isModified(ctx, bazName, "foo-role")).Should(BeTrue())
	})

	It("should overwrite the conflicting source in the descedants", func() {
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", barName, "bar-role")).Should(BeTrue())
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "bar-role")).Should(Equal(barName))

		makeObject(ctx, "Role", fooName, "bar-role")
		// Add a 500-millisecond gap here to allow updating the cached bar-roles in bar
		// and baz namespaces. Without this, even having 20 seconds in the "Eventually()"
		// funcs below, the test failed with timeout. Guess the reason is that it's
		// constantly getting the cached object.
		time.Sleep(500 * time.Millisecond)
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		Eventually(objectInheritedFrom(ctx, "Role", bazName, "bar-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", barName, "bar-role")).Should(BeTrue())
		Eventually(objectInheritedFrom(ctx, "Role", barName, "bar-role")).Should(Equal(fooName))
	})

	It("should overwrite conflicting source with the top source that can propagate", func() {
		// Create a 'baz-role' in 'foo' that cannot propagate because of the finalizer.
		makeObject(ctx, "Role", fooName, "baz-role")
		Eventually(hasObject(ctx, "Role", fooName, "baz-role")).Should(BeTrue())
		setFinalizer(ctx, fooName, "baz-role", true)
		// Create a 'baz-role' in 'bar' that can propagate.
		makeObject(ctx, "Role", barName, "baz-role")

		// Before the tree is constructed, 'baz-role' shouldn't be overwritten.
		Eventually(hasObject(ctx, "Role", bazName, "baz-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "baz-role")).Should(Equal(""))

		// Construct the tree: foo (root) <- bar <- baz.
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", bazName, "baz-role")).Should(BeTrue())
		// The 'baz-role' in 'baz' should be overwritten by the conflicting one in
		// 'bar' but not 'foo', since the one in 'foo' cannot propagate with
		// finalizer. Add a 500-millisecond gap to allow overwriting the object.
		time.Sleep(500 * time.Millisecond)
		Expect(objectInheritedFrom(ctx, "Role", bazName, "baz-role")).Should(Equal(barName))
	})

	It("should have deletions propagated after crit conditions are removed", func() {
		// Create tree: bar -> foo (root) and make sure foo-role is propagated
		setParent(ctx, barName, fooName)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())

		// Create a critical condition on foo (and also bar by extension)
		brumpfName := createNSName("brumpf")
		fooHier := newOrGetHierarchy(ctx, fooName)
		fooHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritParentMissing)).Should(BeTrue())
		Eventually(hasCondition(ctx, barName, api.CritAncestor)).Should(BeTrue())

		// Delete the object from `foo`, wait until we're sure that it's gone, and then wait a while
		// longer and verify it *isn't* deleted from `bar`, because the critical condition has paused
		// deletions.
		deleteObject(ctx, "Role", fooName, "foo-role")
		Eventually(hasObject(ctx, "Role", fooName, "foo-role")).Should(BeFalse())
		time.Sleep(1 * time.Second) // todo: merge with similar constants elsewhere
		Expect(hasObject(ctx, "Role", barName, "foo-role")()).Should(BeTrue())

		// Resolve the critical condition and verify that the object is deleted
		fooHier = newOrGetHierarchy(ctx, fooName)
		fooHier.Spec.Parent = ""
		updateHierarchy(ctx, fooHier)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeFalse())
	})

	It("shouldn't propagate/delete if the namespace has Crit condition", func() {
		// Set tree as baz -> bar -> foo(root).
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))

		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "bar-role")).Should(Equal(barName))

		// Set foo's parent to a non-existent namespace.
		brumpfName := createNSName("brumpf")
		fooHier := newOrGetHierarchy(ctx, fooName)
		fooHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritParentMissing)).Should(Equal(true))
		Eventually(hasCondition(ctx, barName, api.CritAncestor)).Should(Equal(true))
		Eventually(hasCondition(ctx, bazName, api.CritAncestor)).Should(Equal(true))

		// Set baz's parent to foo and add a new role in foo.
		setParent(ctx, bazName, fooName)
		makeObject(ctx, "Role", fooName, "foo-role-2")

		// Since the sync is frozen, baz should still have bar-role (no deleting).
		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "bar-role")).Should(Equal(barName))
		// baz and bar shouldn't have foo-role-2 (no propagating).
		Eventually(hasObject(ctx, "Role", bazName, "foo-role-2")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", barName, "foo-role-2")).Should(BeFalse())

		// Create the missing parent namespace with one object.
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())
		makeObject(ctx, "Role", brumpfName, "brumpf-role")

		// The Crit conditions should be gone.
		Eventually(hasCondition(ctx, fooName, api.CritParentMissing)).Should(Equal(false))
		Eventually(hasCondition(ctx, barName, api.CritAncestor)).Should(Equal(false))
		Eventually(hasCondition(ctx, bazName, api.CritAncestor)).Should(Equal(false))

		// Everything should be up to date after the Crit conditions are gone.
		Eventually(hasObject(ctx, "Role", fooName, "brumpf-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", fooName, "brumpf-role")).Should(Equal(brumpfName))

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", barName, "foo-role-2")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role-2")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", barName, "brumpf-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "brumpf-role")).Should(Equal(brumpfName))

		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", bazName, "foo-role-2")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role-2")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", bazName, "brumpf-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "brumpf-role")).Should(Equal(brumpfName))

		Eventually(hasObject(ctx, "Role", bazName, "bar-role")).Should(BeFalse())
	})

	It("should set conditions if it's excluded from being propagated, and clear them if it's fixed", func() {
		// Set tree as baz -> bar -> foo(root) and make sure the secret gets propagated.
		setParent(ctx, barName, fooName)
		setParent(ctx, bazName, barName)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role")).Should(Equal(fooName))

		// Make the secret unpropagateable and verify that it disappears.
		setFinalizer(ctx, fooName, "foo-role", true)
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeFalse())
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeFalse())

		// Observe the condition on the source namespace
		want := &api.Condition{
			Code:    api.CannotPropagate,
			Affects: []api.AffectedObject{{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role", Namespace: fooName, Name: "foo-role"}},
		}
		Eventually(getCondition(ctx, fooName, api.CannotPropagate)).Should(Equal(want))

		// Fix the problem and verify that the condition vanishes and the secret is propagated again
		setFinalizer(ctx, fooName, "foo-role", false)
		Eventually(hasCondition(ctx, fooName, api.CannotPropagate)).Should(Equal(false))
		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
		Eventually(hasObject(ctx, "Role", bazName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", bazName, "foo-role")).Should(Equal(fooName))
	})

	It("shouldn't delete a descendant source object with the same name if the sync mode is 'Remove'", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Remove)
		// Set tree as bar -> foo(root).
		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", barName, "bar-sec")
		Eventually(hasObject(ctx, "Secret", barName, "bar-sec")).Should(BeTrue())

		// Create an object with the same name in the parent.
		makeObject(ctx, "Secret", fooName, "bar-sec")
		Eventually(hasObject(ctx, "Secret", fooName, "bar-sec")).Should(BeTrue())
		// Give the reconciler some time to remove the object if it's going to.
		time.Sleep(500 * time.Millisecond)
		// The source object in the child shouldn't be deleted since the type has 'Remove' mode.
		Eventually(hasObject(ctx, "Secret", barName, "bar-sec")).Should(BeTrue())
	})
})

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

func modifyRole(ctx context.Context, nsName, roleName string) {
	nnm := types.NamespacedName{Namespace: nsName, Name: roleName}
	role := &v1.Role{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, role)).Should(Succeed())

	labels := role.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["modify"] = "make-a-change"
	role.SetLabels(labels)
	ExpectWithOffset(1, k8sClient.Update(ctx, role)).Should(Succeed())
}

func setFinalizer(ctx context.Context, nsName, roleName string, set bool) {
	nnm := types.NamespacedName{Namespace: nsName, Name: roleName}
	role := &v1.Role{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nnm, role)).Should(Succeed())
	if set {
		role.ObjectMeta.Finalizers = []string{"example.com/foo"}
	} else {
		role.ObjectMeta.Finalizers = nil
	}
	ExpectWithOffset(1, k8sClient.Update(ctx, role)).Should(Succeed())
}

func isModified(ctx context.Context, nsName, roleName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args.
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: roleName}
		role := &v1.Role{}
		// Even if `isModified` is always called after `hasObject`, we still use `Eventually`
		// here to make sure there's no weird case of failure when the object does exist. This
		// will not increase the test time since it will pass immediately if it succeeds.
		EventuallyWithOffset(1, func() error {
			return k8sClient.Get(ctx, nnm, role)
		}).Should(Succeed())

		labels := role.GetLabels()
		_, ok := labels["modify"]
		return ok
	}
}

func removeRole(ctx context.Context, nsName, roleName string) {
	role := &v1.Role{}
	role.Name = roleName
	role.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Delete(ctx, role)).Should(Succeed())
}
