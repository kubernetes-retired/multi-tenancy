package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg"
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

	It("Should propagate different types", func(){
		// ignore Secret in case this demo is run twice and the secret has been set to propagate 
		MustRun("kubectl hns config set-type --apiVersion v1 --kind Secret ignore")
		MustRun("kubectl create ns", nsOrg)
		MustRun("kubectl hns create", nsTeamA, "-n", nsOrg)
		MustRun("kubectl hns create", nsTeamB, "-n", nsOrg)
		MustRun("kubectl create ns", nsService1)
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		MustRun("kubectl -n", nsTeamB, "create secret generic my-creds --from-literal=password=iamteamb")
		// wait 2 seconds to give time for secret to propogate if it was to
		time.Sleep(2*time.Second)
		// secret does not show up in service-1 because we haven’t configured HNC to propagate secrets in HNCConfiguration.
		RunShouldNotContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")
		MustRun("kubectl hns config set-type --apiVersion v1 --kind Secret propagate")
		// this command is not needed here, just to check that user can run it without error
		MustRun("kubectl get hncconfiguration config -oyaml")
		RunShouldContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")

		// if we move the service back to team-a, the secret disappears because we haven’t created it there:
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		RunShouldContain(nsService1, 2, "kubectl hns describe", nsTeamA)
		RunShouldNotContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")
	})
})
