package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("HNC should delete and create a new Rolebinding instead of updating it", func() {

	const (
		nsParent = "parent"
		nsChild  = "child"
	)

	BeforeEach(func() {
		CheckHNCPath()
		CleanupNamespaces(nsParent, nsChild)
	})

	AfterEach(func() {
		CleanupNamespaces(nsParent, nsChild)
		RecoverHNC()
	})

	It("Should delete and create a Rolebinding when HNC is undeployed - issue #798", func() {
		// NOTE: THERE IS ONE CASE THAT THIS TEST WILL ALWAYS PASS EVEN IF CODE IS BROKEN:
		// After recovering HNC, if nsChild gets reconciled first, the 'admin' rolebinding will
		// be deleted, and the 'edit' rolebinding will be created when nsParent gets reconciled.
		// In this case the rolebinding would not be considered as 'updated' and the test will pass
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl create rolebinding test --clusterrole=admin --serviceaccount=default:default -n", nsParent)
		FieldShouldContain("rolebinding", nsChild, "test", ".roleRef.name", "admin")

		// It takes a while for the pods to actually be deleted - over 60s, in some cases (especially on
		// Kind, I think). But we don't actually need to wait for the pods to be fully deleted - waiting
		// a few moments seems to be fine, and then the terminated pods don't get in the way. I picked
		// 5s fairly arbitrarily, but it works well. Feel free to try lower values it you like.
		//   - aludwin, Sep 2020
		MustRun("kubectl delete deployment --all -n hnc-system")
		time.Sleep(5*time.Second)

		// Replace the source rolebinding
		MustRun("kubectl delete rolebinding test -n", nsParent)
		MustNotRun("kubectl describe rolebinding test -n", nsParent)
		MustRun("kubectl create rolebinding test --clusterrole=edit --serviceaccount=default:default -n", nsParent)
		FieldShouldContain("rolebinding", nsParent, "test", ".roleRef.name", "edit")

		// Restore HNC and verify that the new RB is propagated
		RecoverHNC()
		FieldShouldContain("rolebinding", nsChild, "test", ".roleRef.name", "edit")
	})
})
