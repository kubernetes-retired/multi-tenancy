package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Demo", func() {
	// Test for https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64
	const (
		nsOrg      = "acme-org"
		nsTeamA    = "team-a"
		nsTeamB    = "team-b"
		nsService1 = "service-1"
		nsService2 = "service-2"
		nsService3 = "service-3"
		nsService4 = "service-4"
		nsDev      = "dev"
		nsStaging  = "staging"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsOrg, nsTeamA, nsTeamB, nsService1, nsService2, nsService3, nsService4, nsDev, nsStaging)
	})

	AfterEach(func() {
		CleanupNamespaces(nsOrg, nsTeamA, nsTeamB, nsService1, nsService2, nsService3, nsService4, nsDev, nsStaging)
	})

	It("Should test basic functionalities in demo", func() {
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

	It("Should propagate different types", func() {
		// ignore Secret in case this demo is run twice and the secret has been set to 'Propagate'
		MustRun("kubectl hns config set-type --apiVersion v1 --kind Secret Ignore")
		MustRun("kubectl create ns", nsOrg)
		MustRun("kubectl hns create", nsTeamA, "-n", nsOrg)
		MustRun("kubectl hns create", nsTeamB, "-n", nsOrg)
		MustRun("kubectl create ns", nsService1)
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		MustRun("kubectl -n", nsTeamB, "create secret generic my-creds --from-literal=password=iamteamb")
		// wait 2 seconds to give time for secret to propogate if it was to
		time.Sleep(2 * time.Second)
		// secret does not show up in service-1 because we haven’t configured HNC to propagate secrets in HNCConfiguration.
		RunShouldNotContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")
		MustRun("kubectl hns config set-type --apiVersion v1 --kind Secret Propagate")
		// this command is not needed here, just to check that user can run it without error
		MustRun("kubectl get hncconfiguration config -oyaml")
		RunShouldContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")

		// if we move the service back to team-a, the secret disappears because we haven’t created it there:
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		RunShouldContain(nsService1, 2, "kubectl hns describe", nsTeamA)
		RunShouldNotContain("my-creds", 2, "kubectl -n", nsService1, "get secrets")
	})

	It("Should intergrate hierarchical network policy", func(){
		MustRun("kubectl create ns", nsOrg)
		MustRun("kubectl hns create", nsTeamA, "-n", nsOrg)
		MustRun("kubectl hns create", nsTeamB, "-n", nsOrg)
		MustRun("kubectl hns create", nsService1, "-n", nsTeamA)
		MustRun("kubectl hns create", nsService2, "-n", nsTeamA)
		// create a web service s2 in namespace service-2, and a client pod client-s1 in namespace service-1 that can access this web service
		MustRun("kubectl run s2 -n", nsService2, "--image=nginx --restart=Never --expose --port 80")
		clientArgs := "-i --image=alpine --restart=Never --rm -- sh -c"
		cmdln := "\"wget -qO- --timeout 2 http://s2.service-2\""
		// at least 20 seconds is needed here from experiments 
		RunShouldContain("Welcome to nginx!", 20, 
			"kubectl run client -n", nsService1, clientArgs, cmdln)
		RunShouldContain("Welcome to nginx!", 10, 
			"kubectl run client -n", nsTeamA, clientArgs, cmdln)
		RunShouldContain("Welcome to nginx!", 10, 
			"kubectl run client -n", nsTeamB, clientArgs, cmdln)

		// create a default network policy that blocks any ingress from other namespaces 
		policy := `# temp file created by demo_test.go
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: deny-from-other-namespaces
  namespace: acme-org
spec:
  podSelector:
    matchLabels:
  ingress:
  - from:
    - podSelector: {}`
		
		filename := WriteTempFile(policy)
		defer RemoveFile(filename)
		MustRun("kubectl apply -f", filename)
		// ensure this policy can be propagated to its descendants
		MustRun("kubectl hns config set-type --apiVersion networking.k8s.io/v1 --kind NetworkPolicy Propagate")
		expected := "deny-from-other-namespaces"
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsOrg)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsTeamA)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsTeamB)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsService1)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsService2)

		// Now we’ll see that we can no longer access service-2 from the client in service-1:
		RunErrorShouldContain("wget: download timed out", 10,
			"kubectl run client -n", nsService1, clientArgs, cmdln)
		
		// create a second network policy that will allow all namespaces within team-a to be able to communicate with each other
		policy = `# temp file created by demo_test.go
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: allow-team-a
  namespace: team-a
spec:
  podSelector:
    matchLabels:
  ingress:
  - from:
    - namespaceSelector:
        matchExpressions:
          - key: 'team-a.tree.hnc.x-k8s.io/depth'
            operator: Exists`

		filename2 := WriteTempFile(policy)
		defer RemoveFile(filename2)
		MustRun("kubectl apply -f", filename2)

		expected = "allow-team-a"
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsTeamA)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsService1)
		RunShouldContain(expected, 2, "kubectl get netpol -n", nsService2)

		// Now, we can access the service from other namespaces in team-a, but not outside of it:
		RunShouldContain("Welcome to nginx!", 10, 
			"kubectl run client -n", nsService1, clientArgs, cmdln)
		RunErrorShouldContain("wget: download timed out", 10, 
			"kubectl run client -n", nsTeamB, clientArgs, cmdln)
	})

	It("Should create and delete subnamespaces", func(){
		// set up initial structure
		MustRun("kubectl create ns", nsOrg)
		MustRun("kubectl hns create", nsTeamA, "-n", nsOrg)
		MustRun("kubectl hns create", nsService1, "-n", nsTeamA)
		MustRun("kubectl hns create", nsService2, "-n", nsTeamA)
		MustRun("kubectl hns create", nsService3, "-n", nsTeamA)
		expected := "" + // empty string make go fmt happy
			nsTeamA + "\n" +
			"├── [s] " + nsService1 + "\n" +
			"├── [s] " + nsService2 + "\n" +
			"└── [s] " + nsService3
		RunShouldContain(expected, 2, "kubectl hns tree", nsTeamA)

		// show that you can't re-use a subns name
		MustRun("kubectl hns create", nsDev, "-n", nsService1)
		RunShouldContain("Children:\n  - "+nsDev, 2, "kubectl hns describe", nsService1)
		MustNotRun("kubectl hns create", nsDev, "-n", nsService2)
		RunShouldContain("Children:\n  - "+nsDev, 2, "kubectl hns describe", nsService1)

		// show how to delete a subns correctly
		MustNotRun("kubectl delete ns", nsService3)
		MustRun("kubectl delete subns", nsService3, "-n", nsTeamA)
		// This should not run because service-1 contains its own subnamespace that would be deleted with it,
		MustNotRun("kubectl delete subns", nsService1, "-n", nsTeamA)

		MustRun("kubectl hns set", nsService1, "--allowCascadingDelete")
		MustRun("kubectl delete subns", nsService1, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"└── [s] " + nsService2
		RunShouldContain(expected, 2, "kubectl hns tree", nsTeamA)

		// Show the difference of a subns and regular child ns
		MustRun("kubectl hns create", nsService4, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"├── [s] " + nsService2 + "\n" +
			"└── [s] " + nsService4
		RunShouldContain(expected, 2, "kubectl hns tree", nsTeamA)
		MustRun("kubectl create ns", nsStaging)
		MustRun("kubectl hns set", nsStaging, "--parent", nsService4)
		expected = "" +
			nsService4 + "\n" +
			"└── " + nsStaging
		RunShouldContain(expected, 2, "kubectl hns tree", nsService4)

		// delete subnamespace nsService4, namespace nsStaging won’t be deleted but it will have CritParentMissing condition
		MustRun("kubectl hns set", nsService4, "--allowCascadingDelete")
		MustRun("kubectl delete subns", nsService4, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"└── [s] " + nsService2
		RunShouldContain(expected, 2, "kubectl hns tree", nsTeamA)
		RunShouldContain("CritParentMissing: missing parent", 2, "kubectl hns describe", nsStaging)
	})
})
