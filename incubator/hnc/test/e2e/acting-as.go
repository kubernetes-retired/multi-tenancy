package e2e

import (
	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Acting-as", func() {
	const (
		nsTarget = "target"
		saTeamA  = "team-a"
		saTeamB  = "team-b"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsTarget)
	})

	AfterEach(func() {
		CleanupNamespaces(nsTarget)
	})

	It("should allow acting as different service accounts", func() {
		MustRun("kubectl create ns", nsTarget)
		MustRun("kubectl get ns", nsTarget)
		MustRun("kubectl get sa -n", nsTarget)

		// create service accounts
		MustRun("kubectl -n", nsTarget, "create sa", saTeamA)
		MustRun("kubectl -n", nsTarget, "create sa", saTeamB)

		// fail to create secret
		MustNotRun("kubectl -n", nsTarget, "create secret generic hnc-secret --from-literal=password=testingsa --as system:serviceaccount:"+nsTarget+":"+saTeamA)

		// allow creation of secret using team-a service account
		MustRun("kubectl -n", nsTarget, "create role "+saTeamA+"-sre --verb=create --resource=secrets")
		MustRun("kubectl -n", nsTarget, "create rolebinding "+saTeamA+"-sres --role "+saTeamA+"-sre --serviceaccount="+nsTarget+":"+saTeamA)

		// secret should get created
		MustRun("kubectl -n", nsTarget, "create secret generic hnc-secret --from-literal=password=testingsa --as system:serviceaccount:"+nsTarget+":"+saTeamA)

		// fail to delete secret
		MustNotRun("kubectl -n", nsTarget, "delete secret hnc-secret --as system:serviceaccount:"+nsTarget+":"+saTeamB)

		// allow deletion of secret using team-b service account
		MustRun("kubectl -n", nsTarget, "create role "+saTeamB+"-sre --verb=delete --resource=secrets")
		MustRun("kubectl -n", nsTarget, "create rolebinding "+saTeamB+"-sres --role "+saTeamB+"-sre --serviceaccount="+nsTarget+":"+saTeamB)

		// secret should get deleted
		MustRun("kubectl -n", nsTarget, "delete secret hnc-secret --as system:serviceaccount:"+nsTarget+":"+saTeamB)
	})
})
