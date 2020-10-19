package e2e

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		RunShouldContain("No resources found in "+nsService1, defTimeout, "kubectl -n", nsService1, "get rolebindings")

		// make acme-org the parent of team-a, and team-a the parent of service-1.
		MustRun("kubectl hns set", nsTeamA, "--parent", nsOrg)
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		// Verify and wait for HNC to pick up the change, otherwise the following commands will fail
		RunShouldContain("Parent: "+nsOrg, defTimeout, "kubectl hns describe", nsTeamA)
		RunShouldContain("Parent: "+nsTeamA, defTimeout, "kubectl hns describe", nsService1)
		// This won't work, will be rejected since it would cause a cycle
		MustNotRun("kubectl hns set", nsOrg, "--parent", nsTeamA)
		// verify the tree
		RunShouldContain(nsTeamA, propogationTimeout, "kubectl hns describe", nsOrg)
		RunShouldContain(nsService1, propogationTimeout, "kubectl hns describe", nsTeamA)

		// Now, if we check service-1 again, we’ll see all the rolebindings we expect:
		RunShouldContainMultiple([]string{"hnc.x-k8s.io/inherited-from=acme-org", "hnc.x-k8s.io/inherited-from=team-a"},
			propogationTimeout, "kubectl -n", nsService1, "describe roles")
		RunShouldContainMultiple([]string{"acme-org-sre", "team-a-sre"}, propogationTimeout, "kubectl -n", nsService1, "get rolebindings")

		MustRun("kubectl hns create", nsTeamB, "-n", nsOrg)
		MustRun("kubectl get ns", nsTeamB)
		RunShouldContainMultiple([]string{nsTeamA, nsTeamB, nsService1}, propogationTimeout, "kubectl hns tree", nsOrg)

		// set up roles in team-b
		MustRun("kubectl -n", nsTeamB, "create role", nsTeamB+"-wizard", "--verb=update --resource=deployments")
		MustRun("kubectl -n", nsTeamB, "create rolebinding", nsTeamB+"-wizards", "--role", nsTeamB+"-wizard", "--serviceaccount="+nsTeamB+":default")
		// assign the service to the new team, and check that all the RBAC roles get updated
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		RunShouldNotContain(nsService1, propogationTimeout, "kubectl hns describe", nsTeamA)
		RunShouldContain(nsService1, propogationTimeout, "kubectl hns describe", nsTeamB)
		RunShouldContain(nsTeamB+"-wizard", propogationTimeout, "kubectl -n", nsService1, "get roles")
		RunShouldNotContain(nsTeamA+"-wizard", propogationTimeout, "kubectl -n", nsService1, "get roles")
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
		RunShouldNotContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")
		MustRun("kubectl hns config set-type --apiVersion v1 --kind Secret Propagate --force")
		// this command is not needed here, just to check that user can run it without error
		MustRun("kubectl get hncconfiguration config -oyaml")
		RunShouldContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")

		// if we move the service back to team-a, the secret disappears because we haven’t created it there:
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		RunShouldContain(nsService1, defTimeout, "kubectl hns describe", nsTeamA)
		RunShouldNotContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")
	})

	It("Should intergrate hierarchical network policy", func(){
		GinkgoT().Log("WARNING: IF THIS TEST FAILS, PLEASE CHECK THAT THE NETWORK POLICY IS ENABLED ON THE TEST CLUSTER")

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
		RunShouldContain("Welcome to nginx!", cleanupTimeout, 
			"kubectl run client -n", nsTeamA, clientArgs, cmdln)
		RunShouldContain("Welcome to nginx!", cleanupTimeout, 
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
		MustRun("kubectl hns config set-type --apiVersion networking.k8s.io/v1 --kind NetworkPolicy Propagate --force")
		expected := "deny-from-other-namespaces"
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsOrg)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsTeamA)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsTeamB)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsService1)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsService2)

		// Now we’ll see that we can no longer access service-2 from the client in service-1. If we can,
		// that probably means that network policies aren't enabled on this cluster (e.g. Kind, GKE by
		// default) and we should skip the rest of this test.
		netpolTestStdout := ""
		Eventually(func() error {
			stdout, err := RunCommand("kubectl run client -n", nsService1, clientArgs, cmdln)
			netpolTestStdout = stdout
			return err
		}).Should(Succeed())
		if !strings.Contains(netpolTestStdout, "wget: download timed out") {
			Skip("Basic network policies don't appear to be working; skipping the netpol demo")
		}

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
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsTeamA)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsService1)
		RunShouldContain(expected, defTimeout, "kubectl get netpol -n", nsService2)

		// Now, we can access the service from other namespaces in team-a, but not outside of it:
		RunShouldContain("Welcome to nginx!", cleanupTimeout, 
			"kubectl run client -n", nsService1, clientArgs, cmdln)
		RunErrorShouldContain("wget: download timed out", cleanupTimeout, 
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
		// The subnamespaces takes a bit of time to show up
		RunShouldContain(expected, propogationTimeout, "kubectl hns tree", nsTeamA)

		// show that you can't re-use a subns name
		MustRun("kubectl hns create", nsDev, "-n", nsService1)
		RunShouldContain("Children:\n  - "+nsDev, defTimeout, "kubectl hns describe", nsService1)
		MustNotRun("kubectl hns create", nsDev, "-n", nsService2)
		RunShouldContain("Children:\n  - "+nsDev, defTimeout, "kubectl hns describe", nsService1)

		// show how to delete a subns correctly
		MustNotRun("kubectl delete ns", nsService3)
		MustRun("kubectl delete subns", nsService3, "-n", nsTeamA)
		// This should not run because service-1 contains its own subnamespace that would be deleted with it,
		MustNotRun("kubectl delete subns", nsService1, "-n", nsTeamA)

		MustRun("kubectl hns set", nsService1, "--allowCascadingDeletion")
		MustRun("kubectl delete subns", nsService1, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"└── [s] " + nsService2
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsTeamA)

		// Show the difference of a subns and regular child ns
		MustRun("kubectl hns create", nsService4, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"├── [s] " + nsService2 + "\n" +
			"└── [s] " + nsService4
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsTeamA)
		MustRun("kubectl create ns", nsStaging)
		MustRun("kubectl hns set", nsStaging, "--parent", nsService4)
		expected = "" +
			nsService4 + "\n" +
			"└── " + nsStaging
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsService4)

		// delete subnamespace nsService4, namespace nsStaging won’t be deleted but it will have CritParentMissing condition
		MustRun("kubectl hns set", nsService4, "--allowCascadingDeletion")
		MustRun("kubectl delete subns", nsService4, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"└── [s] " + nsService2
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsTeamA)
		RunShouldContain("CritParentMissing: missing parent", defTimeout, "kubectl hns describe", nsStaging)
	})
})
