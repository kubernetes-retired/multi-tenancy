package e2e

import (
	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Acting-as", func() {
	const (
		parent = "parent"
		child  = "child"
	)

	BeforeEach(func() {
		CleanupNamespaces(parent, child)
	})

	AfterEach(func() {
		CleanupNamespaces(parent, child)
	})

	It("should allow acting as different service accounts", func() {
		// create the namespaces
		MustRun("kubectl create ns", parent)
		MustRun("kubectl create ns", child)

		// fail to set parent
		MustNotRun("kubectl hns set", child, "--parent", parent, "--as system:serviceaccount:"+child+":default")

		// set the proper roles giving the required permissions
		MustRun("kubectl -n", child, "create role "+child+"-hnc-role --verb=get,create,update --resource=HierarchyConfiguration")
		MustRun("kubectl -n", child, "create rolebinding "+child+"-hnc-rolebinding --role "+child+"-hnc-admin --serviceaccount="+child+":default")
		MustRun("kubectl -n", parent, "create role "+parent+"hnc-role --verb=update --resource=HierarchyConfiguration")
		MustRun("kubectl -n", parent, "create rolebinding "+parent+"hnc-rolebinding --role "+parent+"-hnc-admin --serviceaccount="+child+":default")
		MustRun("kubectl -n", child, "create role "+child+"-ns-role --verb=get --resource=namespaces")
		MustRun("kubectl -n", child, "create rolebinding "+child+"-ns-rolebinding --role "+child+"-ns-get --serviceaccount="+child+":default")

		// after setting the above permissions, now able to set the parent
		MustRun("kubectl hns set", child, "--parent", parent, "--as system:serviceaccount:"+child+":default")

	})
})
