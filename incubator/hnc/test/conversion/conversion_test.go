package conversion

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var (
	crds = []string{anchorCRD, hierCRD, configCRD}
)

const (
	certsReadyTime = 20
	// Some reconciliation may take longer so we have it as 7 seconds, e.g. removing
	// v1alpha1 from CRD status.storedVersions after CRD conversion because it can
	// be removed only if all the v1alpha1 CRs are reconciled and converted to v1alpha2.
	crdConversionTime = 7
	propagationTime   = 5

	anchorCRD       = "subnamespaceanchors.hnc.x-k8s.io"
	hierCRD         = "hierarchyconfigurations.hnc.x-k8s.io"
	configCRD       = "hncconfigurations.hnc.x-k8s.io"
	hierSingleton   = "hierarchy"
	configSingleton = "config"

	namspacePrefix = "e2e-conversion-test-"
	nsA            = namspacePrefix + "a"
	nsB            = namspacePrefix + "b"
	nsC            = namspacePrefix + "c"
	nsD            = namspacePrefix + "d"
	nsE            = namspacePrefix + "e"
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
		// CleanupNamespaces uses v1alpha2 which only exists in HNC v0.6. We don't know for sure if v0.6
		// is installed right now but our best bet to clean up namespaces safely is to hope that it is.
		// TODO: make CleanupNamespaces work regardless of whether HNC is running or not (see that
		// function for details).
		CleanupNamespaces(nsA, nsB, nsC, nsD, nsE)

		// Almost all tests start with HNC v0.5 so just start there.
		TearDownHNC(hncFromVersion)
		setupV1alpha1(hncFromVersion)
	})

	AfterEach(func() {
		// Restore to the initial starting point. Clean up namespaces before tearing down HNC to remove
		// finalizers.
		CleanupNamespaces(nsA, nsB, nsC, nsD, nsE)
		TearDownHNC("")
	})

	It("should convert managedBy annotation", func() {
		// Create externally managed namespace
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl annotate ns", nsA, "hnc.x-k8s.io/managedBy=foo")

		// Convert
		setupV1alpha2()

		// Verify correct annotation
		FieldShouldContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managed-by:foo")
	})

	It("should always respect the new managed-by annotation even if both exists before conversion (very unlikely)", func() {
		// Create externally managed namespace with conflicting values
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl annotate ns", nsA, "hnc.x-k8s.io/managedBy=foo")
		MustRun("kubectl annotate ns", nsA, "hnc.x-k8s.io/managed-by=bar") // unknown to v0.5
		// Both should just sit there
		FieldShouldContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managedBy:foo")
		FieldShouldContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managed-by:bar")

		// Convert
		setupV1alpha2()

		// Only the new value should be present
		FieldShouldNotContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managedBy:foo")
		FieldShouldContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managed-by:bar")
	})

	It("should respect the new 'managed-by' annotation AFTER conversion if both exists", func() {
		// Start with v1a2 this time
		setupV1alpha2()

		// Create the namespace with both the correct and obsolete annotations
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl annotate ns", nsA, "hnc.x-k8s.io/managed-by=bar")
		MustRun("kubectl annotate ns", nsA, "hnc.x-k8s.io/managedBy=foo") // deprecated in v0.6

		// Verify that the deprecated version is removed
		FieldShouldNotContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managedBy:foo")
		FieldShouldContain("ns", "", nsA, ".metadata.annotations", "hnc.x-k8s.io/managed-by:bar")
	})

	It("should convert subnamespace anchors and subnamespace-of annotation", func() {
		// Before conversion, create namespace A and a subnamespace B.
		createSampleV1alpha1Subnamespace()

		// Convert
		setupV1alpha2()

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
		createSampleV1alpha1ParentChildTree()

		// Convert
		setupV1alpha2()

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

		// Verify allowCascadingDeletion in the new version.
		FieldShouldContainWithTimeout(hierCRD, nsA, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsA, hierSingleton, ".spec", "allowCascadingDeletion:true")
	})

	It("should still have HC condition if it exists in v1alpha1, but with a new schema", func() {
		// Before conversion, create namespace B with a missing parent A (have to
		// create A first and then delete it because otherwise the webhook will deny
		// setting a non-existing namespace as parent).
		createSampleV1alpha1Tree()
		MustRun("kubectl delete ns", nsA)
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".status.conditions", "code:CritParentMissing")

		// Convert
		setupV1alpha2()

		// Verify conditions in the new version replacing code with type and reason.
		FieldShouldContainWithTimeout(hierCRD, nsB, hierSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".status.conditions", "type:ActivitiesHalted")
		FieldShouldContain(hierCRD, nsB, hierSingleton, ".status.conditions", "reason:ParentMissing")
		FieldShouldNotContain(hierCRD, nsB, hierSingleton, ".status.conditions", "CritParentMissing")
		FieldShouldNotContain(hierCRD, nsB, hierSingleton, ".status.conditions", "code")
	})

	It("should convert HNCConfig", func() {
		// Convert
		setupV1alpha2()

		// Verify default resources in the new version.
		FieldShouldContainWithTimeout(configCRD, "", configSingleton, ".apiVersion", "v1alpha2", crdConversionTime)
		FieldShouldContainMultiple(configCRD, "", configSingleton, ".spec.resources", []string{"roles", "rolebindings"})
	})

	It("should convert HNCConfig resources", func() {
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
  - apiVersion: rbac.authorization.k8s.io/v1 #### RBAC resources
    kind: Role
    mode: propagate
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: RoleBinding
    mode: propagate
  - apiVersion: v1                           #### Other builtin resources we think people will want to use
    kind: Secret
    mode: propagate
  - apiVersion: v1
    kind: ConfigMap
    mode: foobar                             #### this should be converted to 'ignore'
  - apiVersion: v1
    kind: ResourceQuota
    mode: remove
  - apiVersion: v1
    kind: LimitRange
    mode: remove
  - apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    mode: remove
  - apiVersion: v1                           #### Try a custom type too. It will cause a condition since it doesn't exist.
    kind: WrongResource
    mode: remove
`
		MustApplyYAML(cfg)
		// Create a secret in ns A and make sure it's propagated to ns B.
		MustRun("kubectl -n", nsA, "create secret generic my-creds-1 --from-literal=password=iama")
		RunShouldContain("my-creds-1", propagationTime, "kubectl get secrets -n", nsB)

		// Convert
		setupV1alpha2()

		// Verify resources conversion. Look for some text that was legal in v1alpha1 but
		// isn't legal in v1alpha2 as a good first check to ensure conversion.
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "apiVersion")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "kind")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.types", "v1")

		// Check all builtin resources conversions (same order as above)
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "group:rbac.authorization.k8s.io mode:Propagate resource:roles")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "group:rbac.authorization.k8s.io mode:Propagate resource:rolebindings")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "mode:Propagate resource:secrets")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "mode:Ignore resource:configmaps")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "mode:Remove resource:resourcequotas")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "mode:Remove resource:limitranges")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "group:networking.k8s.io mode:Remove resource:networkpolicies")

		// Check WrongResource conversion with ResourceNotFound condition.
		FieldShouldNotContain(configCRD, "", configSingleton, ".status.resources", "wrongresources")
		FieldShouldContain(configCRD, "", configSingleton, ".status.conditions", "message:Resource \"wrongresources\" not found reason:ResourceNotFound")

		// Verify sync mode behavior.
		MustRun("kubectl -n", nsA, "create secret generic my-creds-2 --from-literal=password=iama")
		RunShouldContainMultiple([]string{"my-creds-1", "my-creds-2"}, propagationTime, "kubectl get secrets -n", nsB)
	})

	It("should convert HNCConfig namespaceConditions into just conditions", func() {
		// Create a tree with A as the root and B as the child
		createSampleV1alpha1Tree()
		// Create namespaces C, D, E as the child of namespace A.
		createV1alpha1Child(nsC, nsA)
		createV1alpha1Child(nsD, nsA)
		createV1alpha1Child(nsE, nsA)
		// Delete namespace A to get 4 ParentMissing conditions for B, C, D, E
		MustRun("kubectl delete ns", nsA)

		// Verify v1a1 condition and all 4 affected namespaces.
		FieldShouldContain(configCRD, "", configSingleton, ".status.namespaceConditions", "code:CritParentMissing")
		FieldShouldContainMultiple(configCRD, "", configSingleton, ".status.namespaceConditions", []string{nsB, nsC, nsD, nsE})

		// Convert
		setupV1alpha2()

		// Verify "namespaceConditions" field is gone and converted to conditions.
		FieldShouldNotContain(configCRD, "", configSingleton, ".status", "namespaceConditions")
		FieldShouldContain(configCRD, "", configSingleton, ".status.conditions", "reason:ActivitiesHalted status:True type:NamespaceCondition")
		// Verify the number of affected namespaces is 4.
		FieldShouldContain(configCRD, "", configSingleton, ".status.conditions", "4 namespaces")
		// Verify only first 3 namespaces are listed.
		FieldShouldContainMultiple(configCRD, "", configSingleton, ".status.conditions", []string{nsB, nsC, nsD})
		FieldShouldNotContain(configCRD, "", configSingleton, ".status.conditions", nsE)
	})

	It("should still have HNCConfig condition if it exists in v1alpha1, but with a new schema", func() {
		// Create a tree with A as the root and B as the child
		createSampleV1alpha1Tree()
		// Delete the webhook to apply mulitple configurations for one type.
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
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    mode: remove`
		MustApplyYAML(cfg)
		// Verify v1a1 condition with 'MultipleConfigurationsForOneType' code.
		FieldShouldContain(configCRD, "", configSingleton, ".status.conditions", "code:MultipleConfigurationsForOneType")

		// Convert
		setupV1alpha2()

		// Verify v1a2 condition that replaces code with type and reason.
		FieldShouldNotContain(configCRD, "", configSingleton, ".status.conditions", "code")
		FieldShouldContain(configCRD, "", configSingleton, ".status.conditions", "reason:MultipleConfigurationsForType status:True type:BadConfiguration")
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

		// Verify sync mode conversion.
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "Propagate")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.resources", "propagate")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "Ignore")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.resources", "ignore")
		FieldShouldContain(configCRD, "", configSingleton, ".spec.resources", "Remove")
		FieldShouldNotContain(configCRD, "", configSingleton, ".spec.resources", "remove")
		// Verify sync mode behavior.
		MustRun("kubectl -n", nsA, "create secret generic my-creds-2 --from-literal=password=iama")
		RunShouldContainMultiple([]string{"my-creds-1", "my-creds-2"}, propagationTime, "kubectl get secrets -n", nsB)
	})

	It("should convert inheritedFrom label to 'inherited-from' on propagated objects", func() {
		// Before conversion, create a propagated object in v1alpha1.
		// Create a tree a <- b, where a is the parent.
		createSampleV1alpha1ParentChildTree()

		// Create a role in namespace A that should be propagated to namespace B.
		roleA := "team-a-sre"
		createRole(roleA, nsA)
		// Verify the v1a1 inheritedFrom label - `inheritedFrom`.
		FieldShouldContain("roles", nsB, roleA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)

		// Convert
		setupV1alpha2()

		// Verify the converted v1a2 inheritedFrom label - `inherited-from`.
		FieldShouldContain("roles", nsB, roleA, ".metadata.labels", "hnc.x-k8s.io/inherited-from:"+nsA)
		// Verify the v1a1 label is gone.
		FieldShouldNotContain("roles", nsB, roleA, ".metadata.labels", "inheritedFrom")
	})
	// 3 more corner cases for v1a1 propagated objects with inheritedFrom label
	// during conversion:
	// 1) It may not clear v1a1 propagated objects if the source object is removed during conversion;
	// 2) It may not clear v1a1 propagated objects if the resource mode is changed from 'propagate' to 'remove' during conversion;
	// 3) It should not clear previously propagated objects after conversion if the resource mode was 'ignore'.
	// The first two are possible race conditions and we hacked it in the tests
	// below to make it consistent. The 3rd is always the case. We will add these
	// to the user guide - https://github.com/kubernetes-sigs/multi-tenancy/issues/1210
	It("may not clear v1a1 propagated objects if the source object is removed during conversion", func() {
		// Create a role, let it propagate. Delete the role right before conversion
		// (hacked by deleting the v1a1 deployment first to make it consistent).
		// Create a tree a <- b, where a is the parent.
		createSampleV1alpha1ParentChildTree()
		roleA := "team-a-sre"
		createRole(roleA, nsA)
		// Verify the v1a1 inheritedFrom label - `inheritedFrom`.
		FieldShouldContain("roles", nsB, roleA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
		// Delete the deployment before deleting the source to make sure the HNC
		// v1a1 object reconciler won't remove the propagated object in v1a1.
		MustRun("kubectl -n hnc-system delete deployment hnc-controller-manager")
		MustRun("kubectl delete role", roleA, "-n", nsA)

		// Convert
		MustRun("kubectl apply -f ../../manifests/hnc-manager.yaml")
		verifyConversion()

		// Verify the v1a1 propagated object is still there.
		FieldShouldContain("roles", nsB, roleA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
	})
	It("may not clear v1a1 propagated objects if the type mode is changed from 'propagate' to 'remove' during conversion", func() {
		// Set 'Secret' to 'propagate' in v1a1, create a secret and let it propagate.
		// Then change the mode to 'remove' right before conversion (hacked by
		// deleting the v1a1 deployment first to make it consistent).
		setV1alpha1SecretMode("propagate")
		// Create tree a <- b, where a is the parent.
		createSampleV1alpha1ParentChildTree()
		credA := "my-creds"
		createSecret(credA, nsA)
		// Verify the propagated object in b has the v1a1 "inheritedFrom" label.
		FieldShouldContain("secrets", nsB, credA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
		// Delete the deployment before changing the mode so that it makes sure the
		// HNC v1a1 object reconciler won't remove propagated object in v1a1.
		MustRun("kubectl -n hnc-system delete deployment hnc-controller-manager")
		// Delete the validating webhook so we can change the mode after deleting
		// the deployment.
		MustRun("kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration")
		setV1alpha1SecretMode("remove")

		// Convert
		MustRun("kubectl apply -f ../../manifests/hnc-manager.yaml")
		verifyConversion()

		// Verify the v1a1 propagated object is still there.
		FieldShouldContain("secrets", nsB, credA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
	})
	It("should not clear previously propagated objects after conversion if the type mode was 'ignore'", func() {
		// Set 'Secret' to 'propagate' in v1a1, create a secret and let it propagate.
		// Then change the mode to 'ignore'.
		setV1alpha1SecretMode("propagate")
		createSampleV1alpha1ParentChildTree()
		credA := "my-creds"
		createSecret(credA, nsA)
		// Verify the v1a1 inheritedFrom label - `inheritedFrom`.
		FieldShouldContain("secrets", nsB, credA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
		// Update mode to 'ignore' and verify the new mode is working.
		setV1alpha1SecretMode("ignore")
		time.Sleep(1 * time.Second)
		igSecA := "ignored-creds"
		createSecret(igSecA, nsA)
		RunShouldNotContain(igSecA, propagationTime, "kubectl get secrets -n", nsB)

		// Convert
		setupV1alpha2()

		// Verify the v1a1 propagated object is still there.
		FieldShouldContain("secrets", nsB, credA, ".metadata.labels", "hnc.x-k8s.io/inheritedFrom:"+nsA)
		// Double check the ignored source is not propagated.
		RunShouldNotContain(igSecA, propagationTime, "kubectl get secrets -n", nsB)
	})
})

// Install HNC and kubectl plugin with v1alpha1.
func setupV1alpha1(hncVersion string) {
	GinkgoT().Log("Set up v1alpha1")
	MustRun("kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-" + hncVersion + "/hnc-manager.yaml")
	// Wait for the validating webhooks to be ready.
	ensureVWHReady()

	// Verify there's no 'v1alpha2' in all three CRDs for now.
	checkCRDVersionInField("v1alpha2", ".spec", false)
	checkCRDVersionInField("v1alpha2", ".status", false)
}

// Install HNC and kubectl plugin with v1alpha2.
func setupV1alpha2() {
	GinkgoT().Log("Set up v1alpha2")
	// Delete the deployment to force re-pulling the image. Without this line, a cached
	// image may be used with the old IfNotPresent imagePullPolicy from 0.5 deployment.
	// See https://github.com/kubernetes-sigs/multi-tenancy/issues/1025.
	MustRun("kubectl -n hnc-system delete deployment hnc-controller-manager")
	MustRun("kubectl apply -f ../../manifests/hnc-manager.yaml")
	verifyConversion()
}

func verifyConversion() {
	// Wait for the cert rotator to write caBundles in CRD conversion webhooks.
	ensureCRDConvWHReady()

	// Verify CRDs still have 'v1alpha1' in spec.versions but not in status.storedVersions.
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
func ensureVWHReady() {
	MustRunWithTimeout(certsReadyTime, "kubectl create ns check-webhook")
	MustRun("kubectl delete ns check-webhook")
}

func ensureCRDConvWHReady() {
	for _, crd := range crds {
		RunShouldNotContain("caBundle: Cg==", certsReadyTime, "kubectl get crd "+crd+" -oyaml")
	}
}

// createSampleV1alpha1Tree creates a tree with 'a' as the root, 'b' as the child.
func createSampleV1alpha1Tree() {
	MustRun("kubectl create ns", nsA)
	MustRun("kubectl create ns", nsB)
	hierB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: ` + nsB + `
spec:
  parent: ` + nsA + `
`
	MustApplyYAML(hierB)
}

// createSampleV1alpha1Subnamespace creates 'a' and a subnamespace 'b'.
func createSampleV1alpha1Subnamespace() {
	MustRun("kubectl create ns", nsA)
	subnsB := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: SubnamespaceAnchor
metadata:
  name: ` + nsB + `
  namespace: ` + nsA + `
`
	MustApplyYAML(subnsB)
	FieldShouldContain(anchorCRD, nsA, nsB, ".status.status", "ok")
	FieldShouldContain("ns", "", nsB, ".metadata.annotations", "hnc.x-k8s.io/subnamespaceOf:"+nsA)
}

// createSampleV1alpha1ParentChildTree creates 'a' and a child 'b'.
func createSampleV1alpha1ParentChildTree() {
	MustRun("kubectl create ns", nsA)
	createV1alpha1Child(nsB, nsA)
	FieldShouldContain(hierCRD, nsA, hierSingleton, ".status.children", nsB)
	FieldShouldContain(hierCRD, nsB, hierSingleton, ".spec.parent", nsA)
}

// createV1alpha1Child creates a namespace and sets it as a child of another.
func createV1alpha1Child(cnm, pnm string) {
	MustRun("kubectl create ns", cnm)
	hierChild := `# temp file created by conversion_test.go
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: ` + cnm + `
spec:
  parent: ` + pnm + `
`
	MustApplyYAML(hierChild)
	FieldShouldContain(hierCRD, pnm, hierSingleton, ".status.children", cnm)
	FieldShouldContain(hierCRD, cnm, hierSingleton, ".spec.parent", pnm)
}

// setV1alpha1SecretMode sets the secret sync mode in v1alpah1.
func setV1alpha1SecretMode(mode string) {
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
    mode: ` + mode + `
`
	MustApplyYAML(cfg)
	// We cannot get the mode for a specific type, so this is a rough check since
	// role and rolebinding always have propagate mode.
	FieldShouldContain(configCRD, "", configSingleton, ".spec.types", mode)
}

// createRole creates a role in a namespace.
func createRole(nm, nsnm string) {
	role := `# temp file created by conversion_test.go
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ` + nm + `
  namespace: ` + nsnm + `
rules:
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - update`
	MustApplyYAML(role)
	RunShouldContain(nm, propagationTime, "kubectl get roles -n", nsnm)
}

// createSecret creates a secret in a namespace.
func createSecret(nm, nsnm string) {
	sec := `# temp file created by conversion_test.go
apiVersion: v1
data:
  password: aWFtdGVhbWI=
kind: Secret
metadata:
  name: ` + nm + `
  namespace: ` + nsnm + `
type: Opaque`
	MustApplyYAML(sec)
	RunShouldContain(nm, propagationTime, "kubectl get secrets -n", nsnm)
}
