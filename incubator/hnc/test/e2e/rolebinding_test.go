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

	FIt("Should delete and create a Rolebinding when HNC is undeployed - issue #798", func() {
		// NOTE: THERE IS ONE CASE THAT THIS TEST WILL ALWAYS PASS EVEN IF CODE IS BROKEN:
		// After recovering HNC, if nsChild gets reconciled first, the 'admin' rolebinding will
		// be deleted, and the 'edit' rolebinding will be created when nsParent gets reconciled.
		// In this case the rolebinding would not be considered as 'updated' and the test will pass
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl create rolebinding test --clusterrole=admin --serviceaccount=default:default -n", nsParent)
		FieldShouldContain("rolebinding", nsChild, "test", ".roleRef.name", "admin")
		MustRun("kubectl delete deployment --all -n hnc-system")
		// The pod might take up to a minite to be deleted, we force the deletion here to save time
		MustRun("kubectl delete pods --all -n hnc-system --grace-period=0 --force")
		RunShouldContain("No resources found", 60, "kubectl get pods -n hnc-system")
		MustRun("kubectl delete rolebinding test -n", nsParent)
		MustNotRun("kubectl describe rolebinding test -n", nsParent)
		MustRun("kubectl create rolebinding test --clusterrole=edit --serviceaccount=default:default -n", nsParent)
		FieldShouldContain("rolebinding", nsParent, "test", ".roleRef.name", "edit")
		RecoverHNC()
		FieldShouldContain("rolebinding", nsChild, "test", ".roleRef.name", "edit")
	})
})
