package test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("Delete-anchor-crd", func() {

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
		mustRun("kubectl apply -f", hncRecoverPath)
	})

	It("should create parent and deletable child, and delete the CRD", func() {
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
})
