package test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("When deleting CRDs", func() {

	hncRecoverPath := os.Getenv("HNC_REPAIR")

	const (
		nsParent = "delete-crd-parent"
		nsChild = "delete-crd-child"
	)

	BeforeEach(func() {
		// we don't want to destroy the HNC without being able to repair it, so skip this test if recovery path not set
		if hncRecoverPath == ""{
			Skip("Environment variable HNC_REPAIR not set. Skipping reocovering HNC.")
		}
	})

	AfterEach(func() {
		cleanupNamespaces(nsParent, nsChild)
		err := tryRun("kubectl apply -f", hncRecoverPath)
		if err != nil {
			GinkgoT().Log("-----------------------------WARNING------------------------------")
			GinkgoT().Logf("WARNING: COULDN'T REPAIR HNC: %v", err)
			GinkgoT().Log("ANY TEST AFTER THIS COULD FAIL BECAUSE WE COULDN'T REPAIR HNC HERE")
			GinkgoT().Log("------------------------------------------------------------------")
			GinkgoT().FailNow()
		}
		// give HNC enough time to repair
		time.Sleep(5 * time.Second)
	})

	It("should not delete subnamespaces", func() {
		// set up
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		// test
		mustRun("kubectl delete customresourcedefinition/subnamespaceanchors.hnc.x-k8s.io")
		// verify
		mustRun("kubectl get ns", nsChild)
	})

	It("should create a rolebinding in parent and propagate to child", func() {
		// set up
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// test
		mustRun("kubectl create rolebinding --clusterrole=view --serviceaccount=default:default -n", nsParent, "foo")
		time.Sleep(1 * time.Second)
		// verify
		mustRun("kubectl get -oyaml rolebinding foo -n", nsChild)
		// test - delete CRD
		mustRun("kubectl delete customresourcedefinition/subnamespaceanchors.hnc.x-k8s.io")
		// Sleeping for 5s to give HNC the chance to delete the RB (but it shouldn't)
		time.Sleep(5 * time.Second)
		// verify
		mustRun("kubectl get -oyaml rolebinding foo -n", nsChild)
	})

	It("should fully delete all CRDs", func() {
		// set up
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl hns create", nsChild, "-n", nsParent)
		// test
		mustRun("kubectl delete crd hierarchyconfigurations.hnc.x-k8s.io")
		time.Sleep(1 * time.Second)
		mustRun("kubectl delete crd subnamespaceanchors.hnc.x-k8s.io")
		mustRun("kubectl delete crd hncconfigurations.hnc.x-k8s.io")
		// Give HNC 10s to have the chance to fully delete everything (5s wasn't enough).
		// Verify that the HNC CRDs are gone (if nothing's printed, then they are).
		runShouldNotContain("hnc", "10s", "kubectl get crd")
	})
})
