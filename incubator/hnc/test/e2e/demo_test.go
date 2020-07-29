package e2e

import (
	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Demo", func() {
	// Test for https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64
	const (
		nsOrg = "acme-org"
		nsTeamA = "team-a"
		nsTeamB = "team-b"
		nsService1 = "service-1"
	)

	BeforeEach(func(){
		CleanupNamespaces(nsOrg, nsTeamA, nsTeamB, nsService1)
	})

	AfterEach(func(){
		CleanupNamespaces(nsOrg, nsTeamA, nsTeamB, nsService1)
	})

	It("Should test basic functionalities in demo", func(){
		MustRun("kubectl create ns", nsOrg)
		MustRun("kubectl create ns", nsTeamA)
		MustRun("kubectl create ns", nsService1)
		MustRun("kubectl -n", nsTeamA, "create role", nsTeamA+"-sre", "--verb=update --resource=deployments")
		MustRun("kubectl -n", nsTeamA, "create rolebinding", nsTeamA+"-sres", "--role", nsTeamA+"-sre", "--serviceaccount="+nsTeamA+":default")
		MustRun("kubectl -n", nsOrg, "create role", nsOrg+"-sre", "--verb=update --resource=deployments")
		MustRun("kubectl -n", nsOrg, "create rolebinding", nsOrg+"-sres", "--role", nsOrg+"-sre", "--serviceaccount="+nsOrg+":default")
		// none of this affects service-1
		RunShouldContain("No resources found in "+nsService1, 1, "kubectl -n", nsService1, "get rolebindings")

		// make acme-org the parent of team-a, and team-a the parent of service-1.
		MustRun("kubectl hns set", nsTeamA, "--parent", nsOrg)
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		// This won't work, will be rejected since it would cause a cycle
		MustNotRun("kubectl hns set", nsOrg, "--parent", nsTeamA)
		// verify the tree
		RunShouldContain(nsTeamA, 5, "kubectl hns describe", nsOrg)
		RunShouldContain(nsService1, 5, "kubectl hns describe", nsTeamA)

		// Now, if we check service-1 again, we’ll see all the rolebindings we expect:
		RunShouldContainMultiple([]string{"hnc.x-k8s.io/inheritedFrom=acme-org", "hnc.x-k8s.io/inheritedFrom=team-a"},
			 5, "kubectl -n", nsService1, "describe roles")
		RunShouldContainMultiple([]string{"Role/acme-org-sre", "Role/team-a-sre"}, 5, "kubectl -n", nsService1, "get rolebindings")

		MustRun("kubectl hns create", nsTeamB, "-n", nsOrg)
		MustRun("kubectl get ns", nsTeamB)
		RunShouldContainMultiple([]string{nsTeamA, nsTeamB, nsService1}, 5, "kubectl hns tree", nsOrg)
		
		// set up roles in team-b
		MustRun("kubectl -n", nsTeamB, "create role", nsTeamB+"-wizard", "--verb=update --resource=deployments")
		MustRun("kubectl -n", nsTeamB, "create rolebinding", nsTeamB+"-wizards", "--role", nsTeamB+"-wizard", "--serviceaccount="+nsTeamB+":default")
		// assign the service to the new team, and check that all the RBAC roles get updated
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		RunShouldNotContain(nsService1, 5, "kubectl hns describe", nsTeamA)
		RunShouldContain(nsService1, 5, "kubectl hns describe", nsTeamB)
		RunShouldContain(nsTeamB+"-wizard", 5, "kubectl -n", nsService1, "get roles")
		RunShouldNotContain(nsTeamA+"-wizard", 5, "kubectl -n", nsService1, "get roles")
	})
})
