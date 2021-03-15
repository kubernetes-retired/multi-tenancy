package e2e

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Quickstart", func() {
	// Tests for the HNC user guide quickstarts
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
		CleanupTestNamespaces()
	})

	AfterEach(func() {
		CleanupTestNamespaces()
	})

	It("Should test basic functionalities in quickstart", func() {
		CreateNamespace(nsOrg)
		CreateNamespace(nsTeamA)
		CreateNamespace(nsService1)
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

		CreateSubnamespace(nsTeamB, nsOrg)
		MustRun("kubectl get ns", nsTeamB)
		RunShouldContainMultiple([]string{nsTeamA, nsTeamB, nsService1}, propogationTimeout, "kubectl hns tree", nsOrg)

		// set up roles in team-b
		MustRun("kubectl -n", nsTeamB, "create role", nsTeamB+"-wizard", "--verb=update --resource=deployments")
		MustRun("kubectl -n", nsTeamB, "create rolebinding", nsTeamB+"-wizards", "--role", nsTeamB+"-wizard", "--serviceaccount="+nsTeamB+":default")

		// Assign the service to a new team. To confirm this has taken effect, use the 'tree' command,
		// not the 'describe' command, since the latter includes recent events that may include the term
		// "service1" when it shouldn't.
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		RunShouldNotContain(nsService1, propogationTimeout, "kubectl hns tree", nsTeamA)
		RunShouldContain(nsService1, propogationTimeout, "kubectl hns tree", nsTeamB)

		// Check that all the RBAC roles get updated
		RunShouldContain(nsTeamB+"-wizard", propogationTimeout, "kubectl -n", nsService1, "get roles")
		RunShouldNotContain(nsTeamA+"-wizard", propogationTimeout, "kubectl -n", nsService1, "get roles")
	})

	It("Should propagate different types", func() {
		// ignore Secret in case this quickstart is run twice and the secret has been set to 'Propagate'
		MustRun("kubectl hns config set-resource secrets --mode Ignore")
		CreateNamespace(nsOrg)
		CreateSubnamespace(nsTeamA, nsOrg)
		CreateSubnamespace(nsTeamB, nsOrg)
		CreateNamespace(nsService1)
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamB)
		MustRun("kubectl -n", nsTeamB, "create secret generic my-creds --from-literal=password=iamteamb")
		// wait 2 seconds to give time for secret to propogate if it was to
		time.Sleep(2 * time.Second)
		// secret does not show up in service-1 because we haven’t configured HNC to propagate secrets in HNCConfiguration.
		RunShouldNotContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")
		MustRun("kubectl hns config set-resource secrets --mode Propagate --force")
		// this command is not needed here, just to check that user can run it without error
		MustRun("kubectl get hncconfiguration config -oyaml")
		RunShouldContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")

		// if we move the service back to team-a, the secret disappears because we haven’t created it there:
		MustRun("kubectl hns set", nsService1, "--parent", nsTeamA)
		RunShouldContain(nsService1, defTimeout, "kubectl hns describe", nsTeamA)
		RunShouldNotContain("my-creds", defTimeout, "kubectl -n", nsService1, "get secrets")
	})

	It("Should integrate hierarchical network policy", func() {
		CreateNamespace(nsOrg)
		CreateSubnamespace(nsTeamA, nsOrg)
		CreateSubnamespace(nsTeamB, nsOrg)
		CreateSubnamespace(nsService1, nsTeamA)
		CreateSubnamespace(nsService2, nsTeamA)

		// create a web service s2 in namespace service-2, and a client pod client-s1 in namespace service-1 that can access this web service
		MustRun("kubectl run s2 -n", nsService2, "--image=nginx --restart=Never --expose --port 80")

		// Ensure that we can access the service from various other namespaces
		const (
			clientCmd  = "kubectl run client -n"
			alpineArgs = "-i --image=alpine --restart=Never --rm -- sh -c"

			// These need to be separate from alpineArgs because RunCommand only understands quoted args
			// if the double-quotes appears at the beginning and end of a single string.
			wgetArgs = "\"wget -qO- --timeout 2 http://s2.service-2\""
		)
		// Up to 20 seconds is needed for the service to first come up from experiments
		RunShouldContain("Welcome to nginx!", 20, clientCmd, nsService1, alpineArgs, wgetArgs)
		RunShouldContain("Welcome to nginx!", defTimeout, clientCmd, nsTeamA, alpineArgs, wgetArgs)
		RunShouldContain("Welcome to nginx!", defTimeout, clientCmd, nsTeamB, alpineArgs, wgetArgs)

		// create a default network policy in the root namespace that blocks any ingress from other namespaces
		policy := `# quickstart_test.go: netpol to block access across namespaces
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

		MustApplyYAML(policy)
		// Enable propagation for netpols and wait for it to get propagated at least to service-1
		MustRun("kubectl hns config set-resource networkpolicies --group networking.k8s.io --mode Propagate --force")
		RunShouldContain("deny-from-other-namespaces", defTimeout, "kubectl get netpol -n", nsService1)

		// Now we’ll see that we can no longer access service-2 from the client in service-1. If we can,
		// that probably means that network policies aren't enabled on this cluster (e.g. Kind, GKE by
		// default) and we should skip the rest of this test.
		//
		// The standard matching functions won't work here because we're looking for a particular error
		// string, but we don't want to fail if we've found it. So use the default timeout (2s) by
		// trying up to three times with a 1s gap in between.
		netpolWorks := false
		for i := 0; !netpolWorks && i < 3; i++ {
			// This command will return a non-nil error if it works correctly
			stdout, _ := RunCommand(clientCmd, nsService1, alpineArgs, wgetArgs)
			if strings.Contains(stdout, "wget: download timed out") {
				netpolWorks = true
			}
			time.Sleep(1 * time.Second)
		}
		if !netpolWorks {
			Skip("Basic network policies don't appear to be working; skipping the netpol quickstart")
		}

		// create a second network policy that will allow all namespaces within team-a to be able to
		// communicate with each other, and wait for it to be propagated to the descendant we want to
		// test.
		policy = `# quickstart_test.go: netpol to allow communication within team-a subtree
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
		MustApplyYAML(policy)
		RunShouldContain("allow-team-a", defTimeout, "kubectl get netpol -n", nsService1)

		// Now, we can access the service from other namespaces in team-a, but not outside of it:
		RunShouldContain("Welcome to nginx!", defTimeout, clientCmd, nsService1, alpineArgs, wgetArgs)
		RunErrorShouldContain("wget: download timed out", defTimeout, clientCmd, nsTeamB, alpineArgs, wgetArgs)
	})

	It("Should create and delete subnamespaces", func() {
		// set up initial structure
		CreateNamespace(nsOrg)
		CreateSubnamespace(nsTeamA, nsOrg)
		CreateSubnamespace(nsService1, nsTeamA)
		CreateSubnamespace(nsService2, nsTeamA)
		CreateSubnamespace(nsService3, nsTeamA)

		expected := "" + // empty string make go fmt happy
			nsTeamA + "\n" +
			"├── [s] " + nsService1 + "\n" +
			"├── [s] " + nsService2 + "\n" +
			"└── [s] " + nsService3
		// The subnamespaces takes a bit of time to show up
		RunShouldContain(expected, propogationTimeout, "kubectl hns tree", nsTeamA)

		// show that you can't re-use a subns name
		CreateSubnamespace(nsDev, nsService1)
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
		CreateSubnamespace(nsService4, nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"├── [s] " + nsService2 + "\n" +
			"└── [s] " + nsService4
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsTeamA)
		CreateNamespace(nsStaging)
		MustRun("kubectl hns set", nsStaging, "--parent", nsService4)
		expected = "" +
			nsService4 + "\n" +
			"└── " + nsStaging
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsService4)

		// delete subnamespace nsService4, namespace nsStaging won’t be deleted but it will have ParentMissing condition
		MustRun("kubectl hns set", nsService4, "--allowCascadingDeletion")
		MustRun("kubectl delete subns", nsService4, "-n", nsTeamA)
		expected = "" +
			nsTeamA + "\n" +
			"└── [s] " + nsService2
		RunShouldContain(expected, defTimeout, "kubectl hns tree", nsTeamA)
		RunShouldContain("ActivitiesHalted (ParentMissing):", defTimeout, "kubectl hns describe", nsStaging)
	})

	It("Should demonstrate exceptions", func() {
		// set up initial structure
		CreateNamespace(nsOrg)
		CreateSubnamespace(nsTeamA, nsOrg)
		CreateSubnamespace(nsTeamB, nsOrg)

		MustRun("kubectl -n", nsOrg, "create secret generic my-secret --from-literal=password=iamacme")
		// allow secret to propagate
		MustRun("kubectl hns config set-resource secrets --mode Propagate --force")
		// check that the secrete has been propagated to both subnamespaces
		RunShouldContain("my-secret", defTimeout, "kubectl -n", nsTeamA, "get secrets")
		RunShouldContain("my-secret", defTimeout, "kubectl -n", nsTeamB, "get secrets")

		// add exceptions annotation
		MustRun("kubectl annotate secret my-secret -n", nsOrg, "propagate.hnc.x-k8s.io/treeSelect=!team-b")
		// check that the secret is no longer accessible from team-b
		RunShouldNotContain("my-secret", defTimeout, "kubectl -n", nsTeamB, "get secrets")

		// delete secret and re-create from the yaml file
		MustRun("kubectl delete secret my-secret -n", nsOrg)
		RunShouldNotContain("my-secret", defTimeout, "kubectl -n", nsTeamA, "get secrets")
		secret := `# quickstart_test.go: a secret with exceptions annotation
apiVersion: v1
kind: Secret
metadata:
  annotations:
    propagate.hnc.x-k8s.io/treeSelect: team-a
  name: my-secret
  namespace: acme-org`
  		MustApplyYAML(secret)
  		RunShouldContain("my-secret", defTimeout, "kubectl -n", nsTeamA, "get secrets")
		RunShouldNotContain("my-secret", defTimeout, "kubectl -n", nsTeamB, "get secrets")
	})
})
