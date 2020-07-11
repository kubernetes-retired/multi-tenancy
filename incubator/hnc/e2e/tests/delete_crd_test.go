package test

import (
	"os"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("Delete-anchor-crd", func() {
	hncRecoverPath := os.Getenv("HNC_REPAIR")
	AfterEach(func() {
		// clean up
		nses := []string{
			"delete-crd-parent",
			"delete-crd-child",
		}
		// Remove all possible objections HNC might have to deleting a network. Make sure it 
		// has cascading deletion so we can delete any of its subnamespace descendants, and 
		// make sure that it's not a subnamespace itself so we can delete it directly.
		for _, ns := range nses {
			tryRun("kubectl hns set", ns, "-a")
			tryRun("kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespaceOf-")
			tryRun("kubectl delete ns", ns)
		}
		mustRun("kubectl apply -f", hncRecoverPath)
	})

	It("should create parent and deletable child, and delete the CRD", func() {
		// we don't want to destroy the HNC without being able to repair it, so skip this test if recovery path not set
		if hncRecoverPath == ""{
			Skip("Environment variable HNC_REPAIR not set. Skipping reocovering HNC.")
		}
		// set up
		mustRun("kubectl create ns delete-crd-parent")
		mustRun("kubectl hns create delete-crd-child -n delete-crd-parent")
		// test
		mustRun("kubectl delete customresourcedefinition/subnamespaceanchors.hnc.x-k8s.io")
		// verify
		mustRun("kubectl get ns delete-crd-child")
	})
})
