package test

import (
	"time"

	. "github.com/onsi/ginkgo"
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
		cleanupNamespaces(nsParent, nsChild, nsSub1, nsSub2, nsSub1Sub1, nsSub2Sub1, 
			nsSubSub2, nsSubChild, nsSubSubChild)
	})

	AfterEach(func() {
		cleanupNamespaces(nsParent, nsChild, nsSub1, nsSub2, nsSub1Sub1, nsSub2Sub1, 
			nsSubSub2, nsSubChild, nsSubSubChild)
	}) 

	It("Should remove obsolete conditions CannotPropagateObject and CannotUpdateObject - issue #328", func() {
		// Setting up hierarchy with rolebinding that HNC doesn't have permission to copy.
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// cluster-admin is the highest-powered ClusterRole and HNC is missing some of
		// its permissions, so it cannot propagate it.
		mustRun("kubectl create rolebinding cluster-admin-rb -n", nsParent, 
			"--clusterrole='cluster-admin' --serviceaccount="+nsParent+":default")
		// Tree should show CannotPropagateObject in nsParent and CannotUpdateObject in nsChild
		runShouldContainMultiple([]string{"1) CannotPropagateObject", "2) CannotUpdateObject"}, 1, "kubectl hns tree", nsParent)
		// Remove the child and verify that the condition is gone
		mustRun("kubectl hns set", nsChild, "--root")
		// There should no longer be any conditions in parent and child
		runShouldContain("No conditions", 1, "kubectl hns describe", nsParent)
		runShouldContain("No conditions", 1, "kubectl hns describe", nsChild)
	})

	It("Should set SubnamespaceAnchorMissing condition if the anchor is missing - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		mustRun("kubectl create ns", nsParent)
		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation 
		mustRun("kubectl create ns", nsSub1)
		mustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		mustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespaceOf=" + nsParent)
		// If the subnamespace doesn't allow cascadingDelete and the anchor is missing in the parent namespace, it should have 'SubnamespaceAnchorMissing' condition while its descendants shoudn't have any conditions."
		// Expected: 'sub1' namespace is not deleted and should have 'SubnamespaceAnchorMissing' condition; no other conditions."
		runShouldContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should unset SubnamespaceAnchorMissing condition if the anchor is re-added - issue #501", func(){
		// set up
		mustRun("kubectl create ns", nsParent)
		// create a subnamespace without anchor by creating a full namespace with SubnamespaceOf annotation 
		mustRun("kubectl create ns", nsSub1)
		mustRun("kubectl hns set", nsSub1, "--parent", nsParent)
		mustRun("kubectl annotate ns", nsSub1, "hnc.x-k8s.io/subnamespaceOf=" + nsParent)
		runShouldContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
		// If the anchor is re-added, it should unset the 'SubnamespaceAnchorMissing' condition in the subnamespace.
		// Operation: recreate the 'sub1' subns in 'parent' - kubectl hns create sub1 -n parent
		// Expected: no conditions.
		mustRun("kubectl hns create", nsSub1, "-n", nsParent)
		runShouldNotContain("SubnamespaceAnchorMissing", 1, "kubectl get hierarchyconfigurations.hnc.x-k8s.io -n", nsSub1, "-o yaml")
	})

	It("Should cascading delete immediate subnamespaces if the anchor is deleted and the subnamespace allows cascadingDelete - issue #501", func() {
		// set up
		mustRun("kubectl create ns", nsParent)
		// Creating the a branch of subnamespace
		mustRun("kubectl hns create", nsSub1, "-n", nsParent)
		mustRun("kubectl hns create", nsSub1Sub1, "-n", nsSub1)
		mustRun("kubectl hns create", nsSub2Sub1, "-n", nsSub1)
		// If the subnamespace allows cascadingDelete and the anchor is deleted, it should cascading delete all immediate subnamespaces.
		// Operation: 1) allow cascadingDelete in 'ochid1' - kubectl hns set sub1 --allowCascadingDelete=true
		// 2) delete 'sub1' subns in 'parent' - kubectl delete subns sub1 -n parent
		// Expected: 'sub1', 'sub1-sub1', 'sub2-sub1' should all be gone
		mustRun("kubectl hns set", nsSub1, "--allowCascadingDelete=true")
		mustRun("kubectl delete subns", nsSub1, "-n", nsParent)
		runShouldNotContainMultiple([]string {nsSub1, nsSub1Sub1, nsSub2Sub1}, 1, "kubectl hns tree", nsParent)
	})

	It("Should cascading delete all subnamespaces if the parent is deleted and allows cascadingDelete - issue #501", func() {
		// Setting up a 3-level tree with 'parent' as the root
		mustRun("kubectl create ns", nsParent)
		// Creating the 1st branch of subnamespace
		mustRun("kubectl hns create", nsSub1, "-n", nsParent)
		mustRun("kubectl hns create", nsSub1Sub1, "-n", nsSub1)
		mustRun("kubectl hns create", nsSub2Sub1, "-n", nsSub1)
		// Creating the 2nd branch of subnamespaces
		mustRun("kubectl hns create", nsSub2, "-n", nsParent)
		mustRun("kubectl hns create", nsSubSub2, "-n", nsSub2)
		// Creating the 3rd branch of a mix of full and subnamespaces
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		mustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// If the parent namespace allows cascadingDelete and it's deleted, all its subnamespaces should be cascading deleted.
		// Operation: 1) allow cascadingDelete in 'parent' - kubectl hns set parent --allowCascadingDelete=true
		// 2) delete 'parent' namespace - kubectl delete ns parent
		// Expected: only 'fullchild' and 'sub-fullchild' should be left and they should have CRIT_ conditions related to missing 'parent'
		mustRun("kubectl hns set", nsParent, "--allowCascadingDelete=true")
		mustRun("kubectl delete ns", nsParent)
		mustNotRun("kubectl hns tree", nsParent)
		mustNotRun("kubectl hns tree", nsSub1)
		mustNotRun("kubectl hns tree", nsSub2)
		runShouldContain("CritParentMissing: missing parent", 1, "kubectl hns tree", nsChild)
		runShouldContain("CritAncestor", 1, "kubectl hns describe", nsSubChild)
	})

	It("Should clear CannotUpdate conditions in descendants when a hierarchy changes - issue #605", func() {
		// Setting up hierarchy with rolebinding that HNC doesn't have permission to copy
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl create ns", nsSubChild)
		mustRun("kubectl create ns", nsSubSubChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		mustRun("kubectl hns set", nsSubChild, "--parent", nsChild)
		mustRun("kubectl hns set", nsSubSubChild, "--parent", nsSubChild)
		// cluster-admin is the highest-powered ClusterRole and HNC is missing some of its permissions, so it cannot propagate it.
		mustRun("kubectl create rolebinding cluster-admin-rb -n", nsParent, "--clusterrole='cluster-admin' --serviceaccount="+nsParent+":default")
		// We put 30s sleep here because - before fixing issue #605, we should see the object gets reconciled after around 8s, 
		// triggered by controller-runtime, with this sleep time. After fixing this issue, the obsolete condition should be cleared
		// immediately 
		time.Sleep(30 * time.Second)
		// Tree should show CannotPropagateObject in parent and CannotUpdateObject in child and subchild
		runShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		runShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
		runShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubChild)
		// Remove the grandchild to avoid removing CannotPropagate condition in parent or CannotUpdate condition in child.
		// Verify that the conditions in grandchild and greatgrandchild are gone.
		mustRun("kubectl hns set", nsSubChild, "--root")
		// We should see sub-child has conditions, and should see sub-sub-child has no conditions immediately after the fix.
		runShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubChild)
		runShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsSubSubChild)
		// There should still be CannotPropagate condition in parent and CannotUpdate condition in child
		runShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		runShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
	})

	It("Should have CritParentMissing condition when parent namespace is deleted - issue #716", func(){
		// Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'"
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Test: Remove parent namespace 'a'
		// Expected: b should have 'CritParentMissing' condition
		mustRun("kubectl delete ns", nsParent)
		runShouldContain("CritParentMissing", 1, "kubectl hns describe", nsChild)
	})

	It("Should delete namespace with CritParentMissing condition - issue #716", func(){
		// Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'"
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// create and verify CritParentMissing condition
		mustRun("kubectl delete ns", nsParent)
		runShouldContain("CritParentMissing", 1, "kubectl hns describe", nsChild)
		// test: delete namespace 
		mustRun("kubectl delete ns", nsChild)
	})

	It("Should not delete a parent of a subnamespace if allowCascadingDelete is not set -issue #716", func(){
		// Setting up a 2-level tree 
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		// test
		mustNotRun("kubectl delete ns", nsParent)
	})

	It("Should delete leaf subnamespace without setting allowCascadingDelete - issue #716", func(){
		// Setting up a 2-level tree 
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		// test
		mustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should not delete a non-leaf subnamespace if allowCascadingDelete is not set - issue #716", func(){
		// Setting up a 3-level tree 
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		mustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// Test: remove non-leaf subnamespace with 'allowCascadingDelete' unset.
		// Expected: forbidden because 'allowCascadingDelete'flag is not set
		mustNotRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should delete a subnamespace if it's changed from non-leaf to leaf without setting allowCascadingDelete - issue #716", func(){
		// Setting up a 3-level tree 
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		mustRun("kubectl hns create", nsSubChild, "-n", nsChild)
		// Test: remove leaf subnamespaces with 'allowCascadingDelete' unset.
		// Expected: delete successfully
		mustRun("kubectl delete subns", nsSubChild, "-n", nsChild)
		// Test: delete child subns in parent
		// Expected: delete successfully
		mustRun("kubectl delete subns", nsChild, "-n", nsParent)
	})

	It("Should set and unset CannotPropagateObject/CannotUpdateObject condition - issue 771", func(){
		// set up
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating unpropagatable object; both namespaces should have a condition
		mustRun("kubectl create rolebinding --clusterrole=cluster-admin --serviceaccount=default:default -n", nsParent, "foo")
		// verify
		runShouldContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		runShouldContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
		// Deleting unpropagatable object; all conditions should be cleared
		mustRun("kubectl delete rolebinding -n", nsParent, "foo")
		runShouldNotContain("CannotPropagateObject", 1, "kubectl hns describe", nsParent)
		runShouldNotContain("CannotUpdateObject", 1, "kubectl hns describe", nsChild)
	})

	It("Should propogate admin rolebindings - issue #772", func(){
		// set up
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// Creating admin rolebinding object
		mustRun("kubectl create rolebinding --clusterrole=admin --serviceaccount=default:default -n", nsParent, "foo")
		// Object should exist in the child, and there should be no conditions
		mustRun("kubectl get rolebinding foo -n", nsChild, "-oyaml")
	})
})
