package conversion

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var (
	crds      = []string{anchorCRD, hierCRD, configCRD}
)

const (
	certsReadyTime = 20
	// Some reconciliation may take longer so we have it as 7 seconds, e.g. removing
	// v1alpha1 from CRD status.storedVersions after CRD conversion because it can
	// be removed only if all the v1alpha1 CRs are reconciled and converted to v1alpha2.
	crdConversionTime = 7
	propagationTime = 5

	anchorCRD       = "subnamespaceanchors.hnc.x-k8s.io"
	hierCRD         = "hierarchyconfigurations.hnc.x-k8s.io"
	configCRD       = "hncconfigurations.hnc.x-k8s.io"
	hierSingleton   = "hierarchy"
	configSingleton = "config"

	namspacePrefix = "e2e-conversion-test-"
	nsA            = namspacePrefix + "a"
	nsB            = namspacePrefix + "b"
)

func TestConversion(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"HNC v1alpha2 Conversion Test Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = Describe("Conversion from v1alpha1 to v1alpha2", func() {
	const (
		// hncFromVersion has to be a version using v1alpha1.
		hncFromVersion = "v0.5.0"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsA, nsB)
		// Tear down HNC for both the specified version and at HEAD.
		TearDownHNC(hncFromVersion)
		setupV1alpha1(hncFromVersion)
	})

	AfterEach(func() {
		CleanupNamespaces(nsA, nsB)
		// Only tear down HNC at HEAD, since that's what we just deployed.
		TearDownHNC("")
	})

	It("should convert subnamespace anchors and subnamespace-of annotation", func() {
		// Before conversion, create namespace A and a subnamespace B.
		createSampleV1alpha1Subnamespace()

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify subnamespace anchor status in the new version.
		FieldShouldContainWithTimeout(anchorCRD, nsA, nsB, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "Ok")
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "map[hnc.x-k8s.io/subnamespace-of:"+nsA+"]")
	})

	It("should always respect the new 'subnamespace-of' annotation even if both exists before conversion (very unlikely)", func() {
		// Before conversion, create namespace A and a subnamespace B.
		createSampleV1alpha1Subnamespace()
		// Add `subnamespace-of` annotation, which should be unknown to HNC v0.5.
		MustRun("kubectl annotate ns", nsB, "hnc.x-k8s.io/subnamespace-of=wrongvalue")
		// Both subnamespace annotations should be there.
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespaceOf:"+nsA)
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespace-of:wrongvalue")

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// It should respect the new 'subnamespace-of' annotation even if it's wrong.
		// Verify subnamespace anchor status ('Conflict') in the new version and the
		// subnamespace annotation value ('wrongvalue').
		FieldShouldContainWithTimeout(anchorCRD, nsA, nsB, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "Conflict")
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespace-of:wrongvalue")
	})

	It("should respect the new 'subnamespace-of' annotation AFTER conversion if both exists", func() {
		// Before conversion, create namespace A and a subnamespace B.
		createSampleV1alpha1Subnamespace()

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify subnamespace anchor status (not 'Conflict') in the new version and
		// the 'subnamespace-of' annotation.
		FieldShouldContainWithTimeout(anchorCRD, nsA, nsB, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "Ok")
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "map[hnc.x-k8s.io/subnamespace-of:"+nsA+"]")

		// Add `subnamespaceOf` annotation, which is obsolete in HNC v0.6.
		MustRun("kubectl annotate ns", nsB, "hnc.x-k8s.io/subnamespaceOf=wrongvalue")
		// Still only the new subnamespace annotation with the same value should be
		// there, and the obsolete one should be deleted.
		FieldShouldNotContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespaceOf:")
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespace-of:"+nsA)
	})

	It("should convert HCs with parent", func() {
		// Before conversion, create namespace A and B and set A as the parent of B.
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl create ns", nsB)
		hierA := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: e2e-conversion-test-b
spec:
  parent: e2e-conversion-test-a`
		MustApplyYAML(hierA)

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify the parent spec and the children status in the new version.
		FieldShouldContainWithTimeout(hierCRD, nsA, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsA, hierSingleton, ".status.children", nsB)
		FieldShouldContainWithTimeout(hierCRD, nsB, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".spec.parent", nsA)
	})

	It("should convert HCs allowCascadingDelete to allowCascadingDeletion", func() {
		// Before conversion, create namespace A with allowCascadingDelete.
		MustRun("kubectl create ns", nsA)
		hierA := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: e2e-conversion-test-a
spec:
  allowCascadingDelete: true`
		MustApplyYAML(hierA)
		FieldShouldContain(hierCRD, nsA, hierSingleton, ".spec", "allowCascadingDelete:true")

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify allowCascadingDeletion in the new version.
		FieldShouldContainWithTimeout(hierCRD, nsA, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsA, hierSingleton, ".spec", "allowCascadingDeletion:true")
	})

	It("should still have HC condition if it exists in v1alpha1", func() {
		// Before conversion, create namespace B with a missing parent A (have to
		// create A first and then delete it because otherwise the webhook will deny
		// setting a non-existing namespace as parent).
		createSampleV1alpha1Tree()
		MustRun("kubectl delete ns", nsA)
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".status.conditions", "CritParentMissing")

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify conditions in the new version.
		FieldShouldContainWithTimeout(hierCRD, nsB, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".status.conditions", "CritParentMissing")
	})

	It("should convert HNCConfig", func() {
		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify default types in the new version.
		FieldShouldContainWithTimeout(configCRD, "", configSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContainMultiple(configCRD, "", configSingleton, ".spec.types", []string{"Role", "RoleBinding"})
	})

	It("should convert HNCConfig sync modes", func() {
		// Create a tree with A as the root and B as the child
		createSampleV1alpha1Tree()
		// Delete the webhook to apply unsupported modes in v1alpha1.
		MustRun("kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration")
		// Set 'propagate', 'remove', unknown ('ignore') modes in v1alpha1
		cfg := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HNCConfiguration
metadata:
  name: config
spec:
  types:
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    mode: propagate
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: RoleBinding
    mode: propagate
  - apiVersion: v1
    kind: Secret
    mode: propagate
  - apiVersion: v1
    kind: ResourceQuota
    mode: remove
  - apiVersion: v1
    kind: ConfigMap
    mode: foobar`
		MustApplyYAML(cfg)
		// Create a secret in ns A and make sure it's propagated to ns B.
		MustRun("kubectl -n", nsA, "create secret generic my-creds-1 --from-literal=password=iama")
		RunShouldContain("my-creds-1", propagationTime, "kubectl get secrets -n", nsB)

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify sync mode conversion.
		FieldShouldContain(configCRD, "", configSingleton, ".spec.types", "Propagate")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "propagate")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.types", "Ignore")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "ignore")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.types", "Remove")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "remove")
		// Verify sync mode behavior.
		MustRun("kubectl -n", nsA, "create secret generic my-creds-2 --from-literal=password=iama")
		RunShouldContainMultiple([]string{"my-creds-1", "my-creds-2"}, propagationTime, "kubectl get secrets -n", nsB)
	})
})

// Install HNC and kubectl plugin with v1alpha1.
func setupV1alpha1(hncVersion string){
	GinkgoT().Log("Set up v1alpha1")
	MustRun("kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-"+hncVersion+"/hnc-manager.yaml")
	// Wait for the validating webhooks to be ready.
	ensureVWHReady()

	// Verify there's no 'v1alpha2' in all three CRDs for now.
	checkCRDVersionInField("v1alpha2", ".spec", false)
	checkCRDVersionInField("v1alpha2", ".status", false)
}

// Install HNC and kubectl plugin with v1alpha2.
func setupV1alpha2(){
	GinkgoT().Log("Set up v1alpha2")
	// Delete the deployment to force re-pulling the image. Without this line, a cached
	// image may be used with the old IfNotPresent imagePullPolicy from 0.5 deployment.
	// See https://github.com/kubernetes-sigs/multi-tenancy/issues/1025.
	MustRun("kubectl -n hnc-system delete deployment hnc-controller-manager")
	MustRun("kubectl apply -f ../../manifests/hnc-manager.yaml")
	// Wait for the cert rotator to write caBundles in CRD conversion webhooks.
	ensureCRDConvWHReady()
}

// Verify CRDs still have 'v1alpha1' in spec.versions but not in status.storedVersions.
func verifyCRDConversion(){
	checkCRDVersionInField("v1alpha1", ".spec.versions", true)
	checkCRDVersionInField("v1alpha2", ".spec.versions", true)
	checkCRDVersionInField("v1alpha1", ".status.storedVersions", false)
	checkCRDVersionInField("v1alpha2", ".status.storedVersions", true)
}

// Check if a specific version exists in the field as expected for a list of CRDs.
func checkCRDVersionInField(version, field string, expected bool) {
	for _, crd := range crds {
		if expected {
			FieldShouldContainWithTimeout("crd", "", crd, field, version, crdConversionTime)
		} else {
			FieldShouldNotContainWithTimeout("crd", "", crd, field, version, crdConversionTime)
		}
	}
}

// Just create a 'check-webhook' namespace to make sure it's not rejected. It
// will be rejected if the validating webhook is not ready.
func ensureVWHReady(){
	MustRunWithTimeout(certsReadyTime, "kubectl create ns check-webhook")
	MustRun("kubectl delete ns check-webhook")
}

func ensureCRDConvWHReady(){
	for _, crd := range crds {
		RunShouldNotContain("caBundle: Cg==", certsReadyTime, "kubectl get crd "+crd+" -oyaml")
	}
}

// createSampleV1alpha1Tree creates a tree with 'a' as the root, 'b' as the child.
func createSampleV1alpha1Tree(){
	MustRun("kubectl create ns e2e-conversion-test-a")
	MustRun("kubectl create ns e2e-conversion-test-b")
	hierB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: e2e-conversion-test-b
spec:
  parent: e2e-conversion-test-a`
	MustApplyYAML(hierB)
}

// createSampleV1alpha1Subnamespace creates 'a' and a subnamespace 'b'.
func createSampleV1alpha1Subnamespace(){
	MustRun("kubectl create ns", nsA)
	subnsB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: SubnamespaceAnchor
metadata:
  name: e2e-conversion-test-b
  namespace: e2e-conversion-test-a`
	MustApplyYAML(subnsB)
	FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "ok")
	FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespaceOf:"+nsA)
}
