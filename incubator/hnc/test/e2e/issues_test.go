package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg"
)

var _ = Describe("Issues", func() {

	const (
		nsParent = "parent"
		nsChild = "child"
		nsSub1 = "sub1"
		nsSub2 = "sub2"
		nsSub1Sub1 = "sub1-sub1"
		nsSub2Sub1 = "sub2-sub1"
		nsSubSub2 = "sub-sub2"
		nsSubChild = "sub-child"
		nsSubSubChild = "sub-sub-child"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsParent, nsChild, nsSub1, nsSub2, nsSub1Sub1, nsSub2Sub1, 
			nsSubSub2, nsSubChild, nsSubSubChild)
	})

	AfterEach(func() {
		CleanupNamespaces(nsParent, nsChild, nsSub1, nsSub2, nsSub1Sub1, nsSub2Sub1, 
			nsSubSub2, nsSubChild, nsSubSubChild)
	}) 

	It("Should not delete full namespace when a faulty anchor is deleted - issue #1149", func() {
		// Setup
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)

		// Wait for subns
		MustRun("kubectl describe ns", nsChild)

		// Remove annotation
		MustRun("kubectl annotate ns", nsChild, "hnc.x-k8s.io/subnamespaceOf-")
		RunShouldNotContain("subnamespaceOf", 1, "kubectl get -oyaml ns", nsChild)

		// Delete anchor
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get subns", nsChild, "-n", nsParent)

		// Verify that namespace still exists
		MustRun("kubectl describe ns", nsChild)
	})

	// Note that this was never actually a problem (only subnamespaces were affected) but it seems
	// like a good thing to test anyway.
	It("Should delete full namespaces with propagated objects - issue #1130", func() {
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		MustRun("kubectl create rolebinding admin-rb -n", nsParent,
			"--clusterrole=admin --serviceaccount="+nsParent+":default")
		MustRun("kubectl delete ns", nsChild)
	})

	It("Should delete subnamespaces with propagated objects - issue #1130", func() {
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl create rolebinding admin-rb -n", nsParent,
			"--clusterrole=admin --serviceaccount="+nsParent+":default")
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should remove obsolete conditions CannotPropagateObject and CannotUpdateObject - issue #328", func() {
		// Setting up hierarchy with rolebinding that HNC doesn't have permission to copy.
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// cluster-admin is the highest-powered ClusterRole and HNC is missing some of
		// its permissions, so it cannot propagate it.
		MustRun("kubectl create rolebinding cluster-admin-rb -n", nsParent, 
			"--clusterrole='cluster-admin' --serviceaccount="+nsParent+":default")
		// Tree should show CannotPropagateObject in nsParent and CannotUpdateObject in nsChild
		RunShouldContainMultiple([]string{"1) CannotPropagateObject", "2) CannotUpdateObject"}, 1, "kubectl hns tree", nsParent)
		// Remove the child and verify that the condition is gone
		MustRun("kubectl hns set", nsChild, "--root")
		// There should no longer be any conditions in parent and child
		RunShouldContain("No conditions", 1, "kubectl hns describe", nsParent)
		RunShouldContain("No conditions", 1, "kubectl hns describe", nsChild)
	})

	It("Should set SubnamespaceAnchorMissing condition if the anchor is missing - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		MustRun("kubectl create ns", nsParent)
		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation 
		MustRun("kubectl create ns", nsSub1)
		MustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		MustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespaceOf=" + nsParent)
		// If the subnamespace doesn't allow cascadingDelete and the anchor is missing in the parent namespace, it should have 'SubnamespaceAnchorMissing' condition while its descendants shoudn't have any conditions."
		// Expected: 'sub1' namespace is not deleted and should have 'SubnamespaceAnchorMissing' condition; no other conditions."
		RunShouldContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should unset SubnamespaceAnchorMissing condition if the anchor is re-added - issue #501", func(){
		// set up
		MustRun("kubectl create ns", nsParent)
		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation 
		MustRun("kubectl create ns", nsSub1)
		MustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		MustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespaceOf=" + nsParent)
		RunShouldContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
		// If the anchor is re-added, it should unset the 'SubnamespaceAnchorMissing' condition in the subnamespace.
		// Operation: recreate the 'sub1' subns in 'parent' - kubectl hns create sub1 -n parent
		// Expected: no conditions.
		MustRun("kubectl hns create", nsSub1, "-n", nsParent)
		RunShouldNotContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should cascading delete immediate subnamespaces if the anchor is deleted and the subnamespace allows cascadingDelete - issue #501", func() {
		// set up
		MustRun("kubectl create ns", nsParent)
		// Creating the a branch of subnamespace
		MustRun("kubectl hns create", nsSub1, "-n", nsParent)
		MustRun("kubectl hns create", nsSub1Sub1, "-n", nsSub1)
		MustRun("kubectl hns create", nsSub2Sub1, "-n", nsSub1)
		// If the subnamespace allows cascadingDelete and the anchor is deleted, it should cascading delete all immediate subnamespaces.
		// Operation: 1) allow cascadingDelete in 'ochid1' - kubectl hns set sub1 --allowCascadingDelete=true
		// 2) delete 'sub1' subns in 'parent' - kubectl delete subns sub1 -n parent
		// Expected: 'sub1', 'sub1-sub1', 'sub2-sub1' should all be gone
		MustRun("kubectl hns set", nsSub1, "--allowCascadingDelete=true")
		MustRun("kubectl delete subns", nsSub1, "-n", nsParent)
		RunShouldNotContainMultiple([]string {nsSub1, nsSub1Sub1, nsSub2Sub1}, 1, "kubectl hns tree", nsParent)
	})

	It("Should cascading delete all subnamespaces if the parent is deleted and allows cascadingDelete - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		MustRun("kubectl create ns", nsParent)
		// Creating the 1st branch of subnamespace
		MustRun("kubectl hns create", nsSub1, "-n", nsParent)
		MustRun("kubectl hns create", nsSub1Sub1, "-n", nsSub1)
		MustRun("kubectl hns create", nsSub2Sub1, "-n", nsSub1)
		// Creating the 2nd branch of subnamespaces
		MustRun("kubectl hns create", nsSub2, "-n", nsParent)
		MustRun("kubectl hns create", nsSubSub2, "-n", nsSub2)
		// Creating the 3rd branch of a mix of full and subnamespaces
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		MustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// If the parent namespace allows cascadingDelete and it's deleted, all its subnamespaces should be cascading deleted.
		// Operation: 1) allow cascadingDelete in 'parent' - kubectl hns set parent --allowCascadingDelete=true
		// 2) delete 'parent' namespace - kubectl delete ns parent
		// Expected: only 'fullchild' and 'sub-fullchild' should be left and they should have CRIT_ conditions related to missing 'parent'
		MustRun("kubectl hns set", nsParent, "--allowCascadingDelete=true")
		MustRun("kubectl delete ns", nsParent)
		MustNotRun("kubectl hns tree", nsParent)
		MustNotRun("kubectl hns tree", nsSub1)
		MustNotRun("kubectl hns tree", nsSub2)
		RunShouldContain("CritParentMissing: missing parent", 1, "kubectl hns tree", nsChild)
		RunShouldContain("CritAncestor", 1, "kubectl hns describe", nsSubChild)
	})

	It("Should clear CannotUpdate conditions in descendants when a hierarchy changes - issue #605", func() {
		// Setting up hierarchy with rolebinding that HNC doesn't have permission to copy
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl create ns", nsSubChild)
		MustRun("kubectl create ns", nsSubSubChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		MustRun("kubectl hns set", nsSubChild, "--parent", nsChild)
		MustRun("kubectl hns set", nsSubSubChild, "--parent", nsSubChild)
		// cluster-admin is the highest-powered ClusterRole and HNC is missing some of its permissions, so it cannot propagate it.
		MustRun("kubectl create rolebinding cluster-admin-rb -n", nsParent, "--clusterrole='cluster-admin' --serviceaccount="+nsParent+":default")
		// We put 30s sleep here because - before fixing issue #605, we should see the object gets reconciled after around 8s, 
		// triggered by controller-runtime, with this sleep time. After fixing this issue, the obsolete condition should be cleared
		// immediately 
		time.Sleep(30 * time.Second)
		// Tree should show CannotPropagateObject in parent and CannotUpdateObject in child and subchild
		RunShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		RunShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
		RunShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubChild)
		// Remove the grandchild to avoid removing CannotPropagate condition in parent or CannotUpdate condition in child.
		// Verify that the conditions in grandchild and greatgrandchild are gone.
		MustRun("kubectl hns set", nsSubChild, "--root")
		// We should see sub-child has conditions, and should see sub-sub-child has no conditions immediately after the fix.
		RunShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubChild)
		RunShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubSubChild)
		// There should still be CannotPropagate condition in parent and CannotUpdate condition in child
		RunShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		RunShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
	})

	It("Should have CritParentMissing condition when parent namespace is deleted - issue #716", func(){
		// Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'"
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Test: Remove parent namespace 'a'
		// Expected: b should have 'CritParentMissing' condition
		MustRun("kubectl delete ns", nsParent)
		RunShouldContain("CritParentMissing", 1, "kubectl hns describe", nsChild)
	})

	It("Should delete namespace with CritParentMissing condition - issue #716", func(){
		// Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'"
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// create and verify CritParentMissing condition
		MustRun("kubectl delete ns", nsParent)
		RunShouldContain("CritParentMissing", 1, "kubectl hns describe", nsChild)
		// test: delete namespace 
		MustRun("kubectl delete ns", nsChild)
	})

	It("Should not delete a parent of a subnamespace if allowCascadingDelete is not set -issue #716", func(){
		// Setting up a 2-level tree 
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		// verify that the namespace has been created 
		MustRun("kubectl get ns", nsChild)
		// test
		MustNotRun("kubectl delete ns", nsParent)
	})

	It("Should delete leaf subnamespace without setting allowCascadingDelete - issue #716", func(){
		// Setting up a 2-level tree 
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		// verify that the namespace has been created 
		MustRun("kubectl get ns", nsChild)
		// test
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should not delete a non-leaf subnamespace if allowCascadingDelete is not set - issue #716", func(){
		// Setting up a 3-level tree 
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// verify that the namespace has been created 
		MustRun("kubectl get ns", nsSubChild)
		// Test: remove non-leaf subnamespace with 'allowCascadingDelete' unset.
		// Expected: forbidden because 'allowCascadingDelete'flag is not set
		MustNotRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should delete a subnamespace if it's changed from non-leaf to leaf without setting allowCascadingDelete - issue #716", func(){
		// Setting up a 3-level tree 
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// verify that the namespace has been created 
		MustRun("kubectl get ns", nsSubChild)
		// Test: remove leaf subnamespaces with 'allowCascadingDelete' unset.
		// Expected: delete successfully
		MustRun("kubectl delete subns", nsSubChild, "-n", nsChild)
		// Test: delete child subns in parent
		// Expected: delete successfully
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should set and unset CannotPropagateObject/CannotUpdateObject condition - issue #771", func(){
		// set up
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating unpropagatable object; both namespaces should have a condition
		MustRun("kubectl create rolebinding --clusterrole=cluster-admin --serviceaccount=default:default -n", nsParent, "foo")
		// verify
		RunShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		RunShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
		// Deleting unpropagatable object; all conditions should be cleared
		MustRun("kubectl delete rolebinding -n", nsParent, "foo")
		RunShouldNotContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		RunShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
	})

	It("Should propogate admin rolebindings - issue #772", func(){
		// set up
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating admin rolebinding object
		MustRun("kubectl create rolebinding --clusterrole=admin --serviceaccount=default:default -n", nsParent, "foo")
		// Object should exist in the child, and there should be no conditions
		MustRun("kubectl get rolebinding foo -n", nsChild, "-oyaml")
	})
})

var _ = Describe("Issues that require repairing HNC", func() {

	const (
		nsParent = "parent"
		nsParent2 = "parent-2"
		nsChild = "child"
		nsSubChild = "sub-child"
	)

	BeforeEach(func() {
		CheckHNCPath()
		CleanupNamespaces(nsParent, nsParent2, nsChild, nsSubChild)

		// Creating a race condition of two parents with the same leaf subns (anchor)
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsParent2)
		// Disabling webhook to generate the race condition
		MustRun("kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration")
		// Creating subns (anchor) 'sub' in both parent and parent2
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent2)
		// Subnamespace child should be created and have parent as the 'subnamespaceOf' annoation value:
		RunShouldContain("subnamespaceOf: " + nsParent, 1, "kubectl get ns", nsChild, "-o yaml")
		// Creating a 'test-secret' in the subnamespace child
		MustRun("kubectl create secret generic test-secret --from-literal=key=value -n", nsChild)
		// subns (anchor) child in parent2 should have 'status: conflict' because it's a bad anchor:
		RunShouldContain("status: conflict", 1, "kubectl get subns", nsChild, "-n", nsParent2, "-o yaml")
		// Enabling webhook again
		RecoverHNC()
	})

	AfterEach(func() {
		CleanupNamespaces(nsParent, nsParent2, nsChild, nsSubChild)
		RecoverHNC()
	})

	It("Should not delete child namespace when deleting a parent namespace with bad anchor - issue #797", func(){
		// Test: remove subns (anchor) in the bad parent 'parent2'
		// Expected: The bad subns (anchor) is deleted successfully but the child is not deleted (still contains the 'test-secret')
		MustRun("kubectl delete subns", nsChild, "-n", nsParent2)
		MustRun("kubectl get secret -n", nsChild)
	})

	It("Should delete child namespace when deleting a parent namespace with good anchor under race condition - issue #797", func(){
		// Test: Remove subns (anchor) in the good parent 'parent'
		// Expected: The subns (anchor) is deleted successfully and the 'child' is also deleted
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get ns", nsChild, "-o yaml")
	})

	It("Should not delete a non-leaf child namespace when deleting a parent namespace with bad anchor - issue #797", func() {
		MustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// Test: remove subns (anchor) in the bad parent 'parent2'
		// Expected: The bad subns (anchor) is deleted successfully but the child is not deleted (still contains the 'test-secret')
		MustRun("kubectl delete subns", nsChild, "-n", nsParent2)
		MustRun("kubectl get secret -n", nsChild)
	})

	It("Should delete a non-leaf child namespace when deleting a parent namespace with good anchor under race condition - issue #797", func(){
		MustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		MustRun("kubectl hns set", nsChild, "-a")
		// Test: Remove subns (anchor) in the good parent 'parent'
		// Expected: The subns (anchor) is deleted successfully and the 'child' is also deleted
		MustRun("kubectl delete subns", nsChild, "-n", nsParent)
		MustNotRun("kubectl get ns", nsChild, "-o yaml")
		MustNotRun("kubectl get ns", nsSubChild, "-o yaml")
	})
})
