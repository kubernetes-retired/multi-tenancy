package e2e

import (
	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Issues", func() {

	const (
		nsParent      = "parent"
		nsChild       = "child"
		nsSub1        = "sub1"
		nsSub2        = "sub2"
		nsSub1Sub1    = "sub1-sub1"
		nsSub2Sub1    = "sub2-sub1"
		nsSubSub2     = "sub-sub2"
		nsSubChild    = "sub-child"
		nsSubSubChild = "sub-sub-child"
	)

	BeforeEach(func() {
		CleanupTestNamespaces()
	})

	AfterEach(func() {
		CleanupTestNamespaces()
	})

	It("Should not delete full namespace when a faulty anchor is deleted - issue #1149", func() {
		// Setup
		CreateNamespace(nsParent)

		CreateSubnamespace(nsChild, nsParent)

		// Wait for subns
		MustRun("kubectl describe ns", nsChild)

		// Remove annotation
		MustRun("kubectl annotate ns", nsChild, "hnc.x-k8s.io/subnamespace-of-")
		RunShouldNotContain("subnamespace-of", 1, "kubectl get -oyaml ns", nsChild)

		// Delete anchor
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get subns", nsChild, "-n", nsParent)

		// Verify that namespace still exists
		MustRun("kubectl describe ns", nsChild)
	})

	// Note that this was never actually a problem (only subnamespaces were affected) but it seems
	// like a good thing to test anyway.
	It("Should delete full namespaces with propagated objects - issue #1130", func() {
		// Set up the structure
		CreateNamespace(nsParent)
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		MustRun("kubectl create rolebinding admin-rb -n", nsParent,
			"--clusterrole=admin --serviceaccount="+nsParent+":default")

		// Wait for the object to be propagated to the child
		MustRun("kubectl get rolebinding admin-rb -n", nsChild, "-oyaml")

		// Successfully delete the child
		MustRun("kubectl delete ns", nsChild)
	})

	It("Should delete subnamespaces with propagated objects - issue #1130", func() {
		// Set up the structure
		CreateNamespace(nsParent)
		CreateSubnamespace(nsChild, nsParent)

		MustRun("kubectl create rolebinding admin-rb -n", nsParent,
			"--clusterrole=admin --serviceaccount="+nsParent+":default")

		// Wait for the object to be propagated to the child
		MustRun("kubectl get rolebinding admin-rb -n", nsChild, "-oyaml")

		// Successfully delete the child
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should set SubnamespaceAnchorMissing condition if the anchor is missing - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		CreateNamespace(nsParent)

		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation
		CreateNamespace(nsSub1)

		MustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		MustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespace-of="+nsParent)
		// If the subnamespace doesn't allow cascadingDeletion and the anchor is missing in the parent namespace, it should have 'SubnamespaceAnchorMissing' condition while its descendants shoudn't have any conditions."
		// Expected: 'sub1' namespace is not deleted and should have 'SubnamespaceAnchorMissing' condition; no other conditions."
		RunShouldContain("reason: SubnamespaceAnchorMissing", defTimeout, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should unset SubnamespaceAnchorMissing condition if the anchor is re-added - issue #501", func() {
		// set up
		CreateNamespace(nsParent)
		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation
		CreateNamespace(nsSub1)
		MustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		MustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespace-of="+nsParent)
		RunShouldContain("reason: SubnamespaceAnchorMissing", defTimeout, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
		// If the anchor is re-added, it should unset the 'SubnamespaceAnchorMissing' condition in the subnamespace.
		// Operation: recreate the 'sub1' subns in 'parent' - kubectl hns create sub1 -n parent
		// Expected: no conditions.
		MustRun("kubectl hns create", nsSub1, "-n", nsParent)
		RunShouldNotContain("reason: SubnamespaceAnchorMissing", defTimeout, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should cascading delete immediate subnamespaces if the anchor is deleted and the subnamespace allows cascadingDeletion - issue #501", func() {
		// set up
		CreateNamespace(nsParent)
		// Creating the a branch of subnamespace
		CreateSubnamespace(nsSub1, nsParent)
		CreateSubnamespace(nsSub1Sub1, nsSub1)
		CreateSubnamespace(nsSub2Sub1, nsSub1)

		// If the subnamespace allows cascadingDeletion and the anchor is deleted, it should cascading delete all immediate subnamespaces.
		// Operation: 1) allow cascadingDeletion in 'ochid1' - kubectl hns set sub1 --allowCascadingDeletion=true
		// 2) delete 'sub1' subns in 'parent' - kubectl delete subns sub1 -n parent
		// Expected: 'sub1', 'sub1-sub1', 'sub2-sub1' should all be gone
		MustRun("kubectl hns set", nsSub1, "--allowCascadingDeletion=true")
		MustRun("kubectl delete subns", nsSub1, "-n", nsParent)
		RunShouldNotContainMultiple([]string{nsSub1, nsSub1Sub1, nsSub2Sub1}, propogationTimeout, "kubectl hns tree", nsParent)
	})

	It("Should cascading delete all subnamespaces if the parent is deleted and allows cascadingDeletion - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		CreateNamespace(nsParent)
		// Creating the 1st branch of subnamespace
		CreateSubnamespace(nsSub1, nsParent)
		CreateSubnamespace(nsSub1Sub1, nsSub1)
		CreateSubnamespace(nsSub2Sub1, nsSub1)
		// Creating the 2nd branch of subnamespaces
		CreateSubnamespace(nsSub2, nsParent)
		CreateSubnamespace(nsSubSub2, nsSub2)
		// Creating the 3rd branch of a mix of full and subnamespaces
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		CreateSubnamespace(nsSubChild, nsChild)
		// If the parent namespace allows cascadingDeletion and it's deleted, all its subnamespaces should be cascading deleted.
		// Operation: 1) allow cascadingDeletion in 'parent' - kubectl hns set parent --allowCascadingDeletion=true
		// 2) delete 'parent' namespace - kubectl delete ns parent
		// Expected: only 'fullchild' and 'sub-fullchild' should be left and they should have ActivitiesHalted conditions related to missing 'parent'
		MustRun("kubectl hns set", nsParent, "--allowCascadingDeletion=true")
		MustRun("kubectl delete ns", nsParent)
		MustNotRun("kubectl hns tree", nsParent)
		MustNotRun("kubectl hns tree", nsSub1)
		MustNotRun("kubectl hns tree", nsSub2)
		RunShouldContain("ActivitiesHalted (ParentMissing)", defTimeout, "kubectl hns tree", nsChild)
		RunShouldContain("ActivitiesHalted (AncestorHaltActivities)", defTimeout, "kubectl hns describe", nsSubChild)
	})

	It("Should have ParentMissing condition when parent namespace is deleted - issue #716", func() {
		// Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'"
		CreateNamespace(nsParent)
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Test: Remove parent namespace 'a'
		// Expected: b should have 'ParentMissing' condition
		MustRun("kubectl delete ns", nsParent)
		RunShouldContain("ActivitiesHalted (ParentMissing)", defTimeout, "kubectl hns describe", nsChild)
	})

	It("Should not delete a parent of a subnamespace if allowCascadingDeletion is not set -issue #716", func() {
		// Setting up a 2-level tree
		CreateNamespace(nsParent)
		CreateSubnamespace(nsChild, nsParent)
		// verify that the namespace has been created
		MustRun("kubectl get ns", nsChild)
		// test
		MustNotRun("kubectl delete ns", nsParent)
	})

	It("Should delete leaf subnamespace without setting allowCascadingDeletion - issue #716", func() {
		// Setting up a 2-level tree
		CreateNamespace(nsParent)
		CreateSubnamespace(nsChild, nsParent)
		// verify that the namespace has been created
		MustRun("kubectl get ns", nsChild)
		// test
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should not delete a non-leaf subnamespace if allowCascadingDeletion is not set - issue #716", func() {
		// Setting up a 3-level tree
		CreateNamespace(nsParent)
		CreateSubnamespace(nsChild, nsParent)
		CreateSubnamespace(nsSubChild, nsChild)
		// verify that the namespace has been created
		MustRun("kubectl get ns", nsSubChild)
		// Test: remove non-leaf subnamespace with 'allowCascadingDeletion' unset.
		// Expected: forbidden because 'allowCascadingDeletion'flag is not set
		MustNotRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should delete a subnamespace if it's changed from non-leaf to leaf without setting allowCascadingDeletion - issue #716", func() {
		// Setting up a 3-level tree
		CreateNamespace(nsParent)
		CreateSubnamespace(nsChild, nsParent)
		CreateSubnamespace(nsSubChild, nsChild)
		// verify that the namespace has been created
		MustRun("kubectl get ns", nsSubChild)
		// Test: remove leaf subnamespaces with 'allowCascadingDeletion' unset.
		// Expected: delete successfully
		MustRun("kubectl delete subns", nsSubChild, "-n", nsChild)
		// make sure the previous operantion is finished, otherwise the next command will fail
		RunShouldNotContain(nsSubChild, propogationTimeout, "kubectl hns tree", nsChild)
		// Test: delete child subns in parent
		// Expected: delete successfully
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should have CannotPropagateObject and CannotUpdateObject events - replacing obsolete issues #328, #605, #771", func() {
		// Set up
		CreateNamespace(nsParent)
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating unpropagatable object; both namespaces should have an event.
		MustRun("kubectl create rolebinding --clusterrole=cluster-admin --serviceaccount=default:default -n", nsParent, "foo")
		// Verify object events
		RunShouldContain("Could not write to destination namespace \""+nsChild+"\"", defTimeout, "kubectl get events -n", nsParent, "--field-selector reason=CannotPropagateObject")
		RunShouldContain("Could not write from source namespace \""+nsParent+"\"", defTimeout, "kubectl get events -n", nsChild, "--field-selector reason=CannotUpdateObject")
	})

	It("Should propogate admin rolebindings - issue #772", func() {
		// set up
		CreateNamespace(nsParent)
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating admin rolebinding object
		MustRun("kubectl create rolebinding --clusterrole=admin --serviceaccount=default:default -n", nsParent, "foo")
		// Object should exist in the child, and there should be no conditions
		MustRun("kubectl get rolebinding foo -n", nsChild, "-oyaml")
	})

	It("should reset allowCascadingDeletion value after the namespace is deleted and recreated - issue #1155", func() {
		// Create a parent namespace and a subnamespace for it.
		CreateNamespace(nsParent)
		MustRun("kubectl get ns", nsParent)
		CreateSubnamespace(nsChild, nsParent)

		// Cascading delete both namespaces.
		MustRun("kubectl hns set", nsParent, "--allowCascadingDeletion")
		FieldShouldContain("hierarchyconfigurations.hnc.x-k8s.io", nsParent, "hierarchy", ".spec", "allowCascadingDeletion:true")
		MustRun("kubectl delete ns", nsParent)

		// Verify deletion.
		MustNotRun("kubectl get ns", nsParent)
		MustNotRun("kubectl get ns", nsChild)

		// Now recreate the parent again.
		CreateNamespace(nsParent)

		// Since nsParent is new, it should not have any kind of hierarchy config in it. So let's ensure
		// that a 'get' fails. We'll get the full YAML so that if it succees, the _contents_ of the
		// config will be in the failure log and we can see what's happened.
		MustNotRun("kubectl get -oyaml hierarchyconfiguration hierarchy -n", nsParent)
	})
})

var _ = Describe("Issues with bad anchors", func() {

	const (
		nsParent   = "parent"
		nsParent2  = "parent-2"
		nsChild    = "child"
		nsSubChild = "sub-child"
	)

	BeforeEach(func() {
		CheckHNCPath()
		CleanupTestNamespaces()

		// Creating a race condition of two parents with the same leaf subns (anchor)
		CreateNamespace(nsParent)
		CreateNamespace(nsParent2)
		// Disabling webhook to generate the race condition
		MustRun("kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration")
		// Creating subns (anchor) 'sub' in both parent and parent2
		CreateSubnamespace(nsChild, nsParent)
		CreateSubnamespace(nsChild, nsParent2)

		// Subnamespace child should be created and have parent as the 'subnamespace-of' annoation value:
		RunShouldContain("subnamespace-of: "+nsParent, defTimeout, "kubectl get ns", nsChild, "-o yaml")
		// Creating a 'test-secret' in the subnamespace child
		MustRun("kubectl create secret generic test-secret --from-literal=key=value -n", nsChild)
		// subns (anchor) child in parent2 should have 'status: Conflict' because it's a bad anchor:
		RunShouldContain("status: Conflict", defTimeout, "kubectl get subns", nsChild, "-n", nsParent2, "-o yaml")
		// Enabling webhook again
		RecoverHNC()
	})

	AfterEach(func() {
		CleanupTestNamespaces()
		RecoverHNC()
	})

	It("Should not delete child namespace when deleting a parent namespace with bad anchor - issue #797", func() {
		// Test: remove subns (anchor) in the bad parent 'parent2'
		// Expected: The bad subns (anchor) is deleted successfully but the child is not deleted (still contains the 'test-secret')
		MustRun("kubectl delete subns", nsChild, "-n", nsParent2)
		MustRun("kubectl get secret -n", nsChild)
	})

	It("Should delete child namespace when deleting a parent namespace with good anchor under race condition - issue #797", func() {
		// Test: Remove subns (anchor) in the good parent 'parent'
		// Expected: The subns (anchor) is deleted successfully and the 'child' is also deleted
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get ns", nsChild, "-o yaml")
	})

	It("Should not delete a non-leaf child namespace when deleting a parent namespace with bad anchor - issue #797", func() {
		CreateSubnamespace(nsSubChild, nsChild)
		// Test: remove subns (anchor) in the bad parent 'parent2'
		// Expected: The bad subns (anchor) is deleted successfully but the child is not deleted (still contains the 'test-secret')
		MustRun("kubectl delete subns", nsChild, "-n", nsParent2)
		MustRun("kubectl get secret -n", nsChild)
	})

	It("Should delete a non-leaf child namespace when deleting a parent namespace with good anchor under race condition - issue #797", func() {
		CreateSubnamespace(nsSubChild, nsChild)
		MustRun("kubectl hns set", nsChild, "-a")
		// Test: Remove subns (anchor) in the good parent 'parent'
		// Expected: The subns (anchor) is deleted successfully and the 'child' is also deleted
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get ns", nsChild, "-o yaml")
		MustNotRun("kubectl get ns", nsSubChild, "-o yaml")
	})
})

var _ = Describe("Issues that require repairing HNC", func() {
	const (
		nsParent = "parent"
		nsChild  = "child"
	)

	BeforeEach(func() {
		CheckHNCPath()
		CleanupTestNamespaces()

		// Ensure we're in a good state
		RecoverHNC()
	})

	AfterEach(func() {
		CleanupTestNamespaces()
		RecoverHNC()
	})

	It("Should allow deletion of namespaces with propagated objects that can't be removed - issue #1214", func() {
		// Create a simple structure and get an object propagated
		CreateNamespace(nsParent)
		CreateNamespace(nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		MustRun("kubectl create role foo --verb get --resource pods -n", nsParent)
		MustRun("kubectl get role foo -n", nsChild)

		// Disable the webhook and put the child into an ActivitiesHalted state so that the secret can't
		// be removed.
		MustRun("kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration")
		MustRun("kubectl hns set", nsChild, "--parent nonexistent")
		RunShouldContain("ActivitiesHalted (ParentMissing)", defTimeout, "kubectl hns tree", nsChild)

		// Restore the webhooks and verify that the namespace can be deleted
		RecoverHNC()
		MustRun("kubectl delete ns", nsChild)
		MustNotRun("kubectl get ns", nsChild)
	})
})
