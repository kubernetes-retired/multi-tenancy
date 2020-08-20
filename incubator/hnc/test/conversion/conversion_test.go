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

	anchorCRD       = "subnamespaceanchors.hnc.x-k8s.io"
	hierCRD         = "hierarchyconfigurations.hnc.x-k8s.io"
	configCRD       = "hncconfigurations.hnc.x-k8s.io"
	hierSingleton   = "hierarchy"
	configSingleton = "config"
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
		namspacePrefix = "e2e-conversion-test-"
		nsA            = namspacePrefix + "a"
		nsB            = namspacePrefix + "b"
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

	It("should convert subnamespace anchors", func() {
		// Before conversion, create namespace A and a subnamespace B.
		MustRun("kubectl create ns", nsA)
		subnsB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: SubnamespaceAnchor
metadata:
  name: e2e-conversion-test-b
  namespace: e2e-conversion-test-a`
		MustApplyYAML(subnsB)
		FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "ok")

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify subnamespace anchor status in the new version.
		FieldShouldContainWithTimeout(anchorCRD, nsA, nsB, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "Ok")
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

	It("should convert HCs with allowCascadingDelete", func() {
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

		// Convert
		setupV1alpha2()

		// Verify CRD conversion.
		verifyCRDConversion()
		// Verify allowCascadingDelete in the new version.
		FieldShouldContainWithTimeout(hierCRD, nsA, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsA, hierSingleton, ".spec", "allowCascadingDelete:true")
	})

	It("should still have HC condition if it exists in v1alpha1", func() {
		// Before conversion, create namespace B with a missing parent A (have to
		// create A first and then delete it because otherwise the webhook will deny
		// setting a non-existing namespace as parent).
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl create ns", nsB)
		hierB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: e2e-conversion-test-b
spec:
  parent: e2e-conversion-test-a`
		MustApplyYAML(hierB)
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
