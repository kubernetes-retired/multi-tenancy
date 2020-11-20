package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("When deleting CRDs", func() {

	const (
		nsParent = "delete-crd-parent"
		nsChild  = "delete-crd-child"
	)

	BeforeEach(func() {
		CheckHNCPath()
		CleanupNamespaces(nsParent, nsChild)
	})

	AfterEach(func() {
		// HNC might be in an arbitrarily bad state - e.g. the CRDs are gone so the reconcilers aren't
		// running anymore, and the admission webhooks are operating off of bad information (e.g. that a
		// subnamespace's annotation has been removed). So delete the VWC so we can clean up
		// effectively; it'll be restored by RecoverHNC anyway.
		if HasHNCPath() {
			MustRun("kubectl delete validatingwebhookconfigurations hnc-validating-webhook-configuration")
		}

		CleanupNamespaces(nsParent, nsChild)
		RecoverHNC()
	})

	It("should not delete subnamespaces", func() {
		// set up
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		MustRun("kubectl get ns", nsChild)
		// delete the CRD
		MustRun("kubectl delete customresourcedefinition/subnamespaceanchors.hnc.x-k8s.io")
		// verify that the child wasn't deleted with the CRDs
		MustRun("kubectl get ns", nsChild)
	})

	It("should create a rolebinding in parent and propagate to child", func() {
		// set up
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl create ns", nsChild)
		MustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// test
		MustRun("kubectl create rolebinding --clusterrole=view --serviceaccount=default:default -n", nsParent, "foo")
		time.Sleep(1 * time.Second)
		// verify
		MustRun("kubectl get -oyaml rolebinding foo -n", nsChild)
		// test - delete CRD
		MustRun("kubectl delete customresourcedefinition/subnamespaceanchors.hnc.x-k8s.io")
		// Sleeping for 5s to give HNC the chance to delete the RB (but it shouldn't)
		time.Sleep(5 * time.Second)
		// verify
		MustRun("kubectl get -oyaml rolebinding foo -n", nsChild)
	})

	It("should fully delete all CRDs", func() {
		// set up
		MustRun("kubectl create ns", nsParent)
		MustRun("kubectl hns create", nsChild, "-n", nsParent)
		// test
		MustRun("kubectl delete crd hierarchyconfigurations.hnc.x-k8s.io")
		time.Sleep(1 * time.Second)
		MustRun("kubectl delete crd subnamespaceanchors.hnc.x-k8s.io")
		MustRun("kubectl delete crd hncconfigurations.hnc.x-k8s.io")
		// Give HNC 10s to have the chance to fully delete everything (5s wasn't enough).
		// Verify that the HNC CRDs are gone (if nothing's printed, then they are).
		RunShouldNotContain("hnc", cleanupTimeout, "kubectl get crd")
	})
})
