package testutils

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The time that Eventually() will keep retrying until timeout
// we use 5 seconds here because some tests require deleting a namespace, and shorter time period might not be enough
const eventuallyTimeout = 5
// The testing label marked on all namespaces created using the testing phase, offering ease when doing cleanups
const testingNamespaceLabel = "hnc.x-k8s.io/testNamespace"

var hncRecoverPath = os.Getenv("HNC_REPAIR")

func FieldShouldContain(resource, ns, nm, field, want string){
	fieldShouldContainMultipleWithTimeout(1, resource, ns, nm, field, []string{want}, eventuallyTimeout)
}

func FieldShouldContainMultiple(resource, ns, nm, field string, want []string){
	fieldShouldContainMultipleWithTimeout(1, resource, ns, nm, field, want, eventuallyTimeout)
}

func FieldShouldContainWithTimeout(resource, ns, nm, field, want string, timeout float64){
	fieldShouldContainMultipleWithTimeout(1, resource, ns, nm, field, []string{want}, timeout)
}

func FieldShouldContainMultipleWithTimeout(resource, ns, nm, field string, want []string, timeout float64){
	fieldShouldContainMultipleWithTimeout(1, resource, ns, nm, field, want, timeout)
}

func fieldShouldContainMultipleWithTimeout(offset int, resource, ns, nm, field string, want []string, timeout float64){
	if ns != "" {
		runShouldContainMultiple(offset+1, want, timeout, "kubectl get", resource, nm, "-n", ns, "-o template --template={{"+field+"}}")
	} else {
		runShouldContainMultiple(offset+1, want, timeout, "kubectl get", resource, nm, "-o template --template={{"+field+"}}")
	}
}

func FieldShouldNotContain(resource, ns, nm, field, want string){
	fieldShouldNotContainMultipleWithTimeout(1, resource, ns, nm, field, []string{want}, eventuallyTimeout)
}

func FieldShouldNotContainMultiple(resource, ns, nm, field string, want []string){
	fieldShouldNotContainMultipleWithTimeout(1, resource, ns, nm, field, want, eventuallyTimeout)
}

func FieldShouldNotContainWithTimeout(resource, ns, nm, field, want string, timeout float64){
	fieldShouldNotContainMultipleWithTimeout(1, resource, ns, nm, field, []string{want}, timeout)
}

func FieldShouldNotContainMultipleWithTimeout(resource, ns, nm, field string, want []string, timeout float64){
	fieldShouldNotContainMultipleWithTimeout(1, resource, ns, nm, field, want, timeout)
}

func fieldShouldNotContainMultipleWithTimeout(offset int, resource, ns, nm, field string, want []string, timeout float64){
	if ns != "" {
		runShouldNotContainMultiple(offset+1, want, timeout, "kubectl get", resource, nm, "-n", ns, "-o template --template={{"+field+"}}")
	} else {
		runShouldNotContainMultiple(offset+1, want, timeout, "kubectl get", resource, nm, "-o template --template={{"+field+"}}")
	}
}

func MustRun(cmdln ...string) {
	mustRunWithTimeout(1, eventuallyTimeout, cmdln...)
}

func MustRunWithTimeout(timeout float64, cmdln ...string) {
	mustRunWithTimeout(1, timeout, cmdln...)
}

func mustRunWithTimeout(offset int, timeout float64, cmdln ...string) {
	EventuallyWithOffset(offset+1, func() error {
		return TryRun(cmdln...)
	}, timeout).Should(Succeed(), "Command: %s", cmdln)
}

func MustNotRun(cmdln ...string) {
	mustNotRun(1, cmdln...)
}

func mustNotRun(offset int, cmdln ...string) {
	ExpectWithOffset(offset+1, func() error {
		return TryRun(cmdln...)
	}).ShouldNot(Equal(nil), "Command: %s", cmdln)
}

func TryRun(cmdln ...string) error {
	stdout, err := RunCommand(cmdln...)
	if err != nil {
		// Add stdout to the error, since it's the error that gets displayed when a test fails and it
		// can be very hard looking at the log to see which failures are intended and which are not.
		err = fmt.Errorf("Error: %s\nOutput: %s", err, stdout)
		GinkgoT().Log("Output (failed): ", err)
	} else {
		GinkgoT().Log("Output (passed): ", stdout)
	}
	return err
}

func TryRunQuietly(cmdln ...string) error {
	_, err := RunCommand(cmdln...)
	return err
}

func RunShouldContain(substr string, seconds float64, cmdln ...string) {
	runShouldContainMultiple(1, []string{substr}, seconds, cmdln...)
}

func RunShouldContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	runShouldContainMultiple(1, substrs, seconds, cmdln...)
}

func runShouldContainMultiple(offset int, substrs []string, seconds float64, cmdln ...string) {
	EventuallyWithOffset(offset+1, func() string {
		missing, err := tryRunShouldContainMultiple(substrs, cmdln...)
		if err != nil {
			return "failed: "+err.Error()
		}
		return missing
	}, seconds).Should(beQuiet(), "Command: %s", cmdln)
}

func RunErrorShouldContain(substr string, seconds float64, cmdln ...string) {
	runErrorShouldContainMultiple(1, []string{substr}, seconds, cmdln...)
}

func RunErrorShouldContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	runErrorShouldContainMultiple(1, substrs, seconds, cmdln...)
}

func runErrorShouldContainMultiple(offset int, substrs []string, seconds float64, cmdln ...string) {
	EventuallyWithOffset(offset+1, func() string {
		missing, err := tryRunShouldContainMultiple(substrs, cmdln...)
		if err == nil {
			return "passed but should have failed"
		}
		return missing
	}, seconds).Should(beQuiet(), "Command: %s", cmdln)
}

func tryRunShouldContainMultiple(substrs []string, cmdln ...string) (string, error) {
		stdout, err := RunCommand(cmdln...)
		GinkgoT().Log("Output: ", stdout)
		return missAny(substrs, stdout), err
}

// If any of the substrs are missing from teststring, returns a string of the form:
//   did not output the expected substring(s): <string1>, <string2>, ...
//   and instead output: teststring
// Otherwise returns the empty string.
func missAny(substrs []string, teststring string) string {
	var missing []string
	for _, substr := range substrs {
		if strings.Contains(teststring, substr) == false {
			missing = append(missing, substr)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	// This looks *ok* if we're only missing a single multiline string, and ok if we're missing
	// multiple single-line strings. It would look awful if we were missing multiple multiline strings
	// but I think that's pretty rare.
	msg := "did not output the expected substring(s): "+strings.Join(missing, ", ")+"\n"
	msg += "and instead output: "+teststring
	return msg
}

func RunShouldNotContain(substr string, seconds float64, cmdln ...string) {
	runShouldNotContain(1, substr, seconds, cmdln...)
}

func runShouldNotContain(offset int, substr string, seconds float64, cmdln ...string) {
	runShouldNotContainMultiple(offset+1, []string{substr}, seconds, cmdln...)
}

func RunShouldNotContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	runShouldNotContainMultiple(1, substrs, seconds, cmdln...)
}

func runShouldNotContainMultiple(offset int, substrs []string, seconds float64, cmdln ...string) {
	EventuallyWithOffset(offset+1, func() string {
		stdout, err := RunCommand(cmdln...)
		if err != nil {
			return "failed: "+err.Error()
		}

		for _, substr := range substrs {
			if strings.Contains(stdout, substr) == true {
				return fmt.Sprintf("included the undesired output %q:\n%s", substr, stdout)
			}
		}

		return ""
	}, seconds).Should(beQuiet(), "Command: %s", cmdln)
}

func MustApplyYAML(s string){
	filename := writeTempFile(s)
	defer removeFile(filename)
	MustRun("kubectl apply -f", filename)
}

// RunCommand passes all arguments to the OS to execute, and returns the combined stdout/stderr and
// and error object. By default, each arg to this function may contain strings (e.g. "echo hello
// world"), in which case we split the strings on the spaces (so this would be equivalent to calling
// "echo", "hello", "world"). If you _actually_ need an OS argument with strings in it, pass it as
// an argument to this function surrounded by double quotes (e.g. "echo", "\"hello world\"" will be
// passed to the OS as two args, not three).
func RunCommand(cmdln ...string) (string, error) {
	var args []string
	for _, subcmdln := range cmdln {
		// Any arg that starts and ends in a double quote shouldn't be split further
		if len(subcmdln) > 2 && subcmdln[0] == '"' && subcmdln[len(subcmdln)-1] == '"' {
			args = append(args, subcmdln[1:len(subcmdln)-1])
		} else {
			args = append(args, strings.Split(subcmdln, " ")...)
		}
	}
	prefix := fmt.Sprintf("[%d] Running: ", time.Now().Unix())
	GinkgoT().Log(prefix, args)
	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}

// CreateNamespace creates the specified namespace with canned testing labels making it easier
// to look up and delete later.
func CreateNamespace( ns string)   {
	MustRun("kubectl create ns", ns)
	labelTestingNs(ns)
}

// CreateSubnamespace creates the specified namespace in the parent namespace with canned testing labels making it easier
// to look up and delete later.
func CreateSubnamespace( ns string,parent string)   {
	MustRun( "kubectl hns create", ns, "-n", parent)
	labelTestingNs(ns)
}

// marks testing namespaces with a label for future search and lookup.
func labelTestingNs(ns string){
	MustRun("kubectl label --overwrite ns", ns,testingNamespaceLabel+"=true")
}

// CleanupTestNamespaces finds the list of namespaces labeled as test namespaces and delegates
// to cleanupNamespaces function.
func CleanupTestNamespaces(){
	nses := []string{}
	EventuallyWithOffset(1, func() error {
		labelQuery := testingNamespaceLabel+"=true"
		out,err:=RunCommand("kubectl get ns -o custom-columns=:.metadata.name --no-headers=true", "-l", labelQuery)
		if err != nil {
			return err
		}
		nses= strings.Split(out,"\n")
		return nil
	}).Should(Succeed(), "while getting list of namespaces to clean up")
	cleanupNamespaces(nses...)
}

// cleanupNamespaces does everything it can to delete the passed-in namespaces. It also uses very
// high timeouts (30s) since this function is often called after HNC has just been reinstalled, and
// it can take a while of HNC to start allowing changes to namespaces again.
//
// TODO: also find a way to remove all finalizers on all HC objects (subns and hierarchy config).
// HNC doesn't put finalizers on namespaces themselves; it blocks namespace deletion by blocking
// deletion of the objects in it, but if HNC is damaged or missing, this can result in namespaces
// never being deleted without admin action.
func cleanupNamespaces(nses ...string) {
	const cleanupTimeout = 30

	// Remove all objections HNC might have to deleting a namespace.
	toDelete := []string{} // exclude missing namespaces
	for _, ns := range nses {
		// Skip any namespace that doesn't actually exist. We only check once (e.g. no retries on
		// errors) but reads are usually pretty reliable.

		// TODO: This check should ideally be removed if we call this function only with a list of valid test namespaces
		// found with the label search. But the RecoverHNC function has to call this with explicit namespaces for a specific test
		if err := TryRunQuietly("kubectl get ns", ns); err != nil {
			continue
		}
		toDelete = append(toDelete, ns)

		// If this is a subnamespace, turn it into a normal namespace so we can delete it directly.
		MustRunWithTimeout(cleanupTimeout, "kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespace-of-")
		// NB: 'subnamespaceOf' is the old subnamespace annotation used in v0.5. We still need to
		// clean up this old annotation because this util func is also used in the API conversion
		// test to clean up namespaces in v0.5.
		//
		// TODO: remove this line after v0.6 branches and we are no longer supporting v1alpha1
		// conversion.
		MustRunWithTimeout(cleanupTimeout, "kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespaceOf-")
	}

	// Now, actually delete them
	for _, ns := range toDelete {
		MustRunWithTimeout(cleanupTimeout, "kubectl delete ns", ns)
	}
}

// TearDownHNC removes CRDs first and then the entire manifest from the current
// change. If a specific HNC version is provided, its manifest will also be
// deleted. It will ensure HNC is cleared at the end.
func TearDownHNC(hncVersion string) {
	// Delete all CRDs first to ensure all finalizers are removed. Since we don't
	// know the version of the current HNC in the cluster so we will try tearing
	// it down twice with the specified version and what's in the HEAD.
	TryRunQuietly("k delete crd subnamespaceanchors.hnc.x-k8s.io")
	TryRunQuietly("k delete crd hierarchyconfigurations.hnc.x-k8s.io")
	TryRunQuietly("k delete crd hncconfigurations.hnc.x-k8s.io")
	TryRunQuietly("kubectl delete -f ../../manifests/hnc-manager.yaml")
	if hncVersion != ""{
		TryRunQuietly("kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-"+hncVersion+"/hnc-manager.yaml")
	}
	// Wait for HNC to be fully torn down (the namespace and the CRDs are gone).
	runShouldNotContain(1, "hnc-system", 10, "kubectl get ns")
	runShouldNotContain(1, ".hnc.x-k8s.io", 10, "kubectl get crd")
}

// CheckHNCPath skips the test if we are not able to successfully call RecoverHNC
func CheckHNCPath() {
	if hncRecoverPath == "" {
		Skip("Environment variable HNC_REPAIR not set. Skipping tests that require repairing HNC.")
	}
}

// HasHNCPath returns true if we'll be able to successfully call RecoverHNC, and false otherwise. 
func HasHNCPath() bool {
	return hncRecoverPath != ""
}

// RecoverHNC assumes that HNC has been damaged in some way and repairs by re-applying its manifest
// and then waiting until very basic functionality is working again.
func RecoverHNC() {
	// Even if a test is skipped because CheckHNCPath returned false, RecoverHNC is often placed in
	// the AfterEach block to clean up after a test is run, and this appears to be called even if the
	// test is skipped (which makes sense, because tests can be skipped at any time and might still
	// require cleanup). So if the path is unset, we skip this function too, otherwise we'll simply
	// delete the existing (healthy) HNC deployment and never repair it.
	if hncRecoverPath == "" {
		return
	}
	// HNC can take a long time (>30s) to recover in some cases if various parts of its deployment are
	// deleted, such as the validating webhook configuration or the CRDs. It appears that deleting the
	// deployment before reapplying the manifests seems to allow HNC to start operating again much
	// faster.
	TryRun("kubectl delete deployment --all -n hnc-system")
	err := TryRun("kubectl apply -f", hncRecoverPath)
	if err != nil {
		GinkgoT().Log("-----------------------------WARNING------------------------------")
		GinkgoT().Logf("WARNING: COULDN'T REPAIR HNC: %v", err)
		GinkgoT().Log("ANY TEST AFTER THIS COULD FAIL BECAUSE WE COULDN'T REPAIR HNC HERE")
		GinkgoT().Log("------------------------------------------------------------------")
		GinkgoT().FailNow()
	}
	// give HNC enough time to repair
	time.Sleep(5 * time.Second)
	// Verify and wait till HNC is fully repaired, sometimes it takes up to 30s. We try to create a
	// subnamespace and wait for it to be created to show that both the validators and reconcilers are
	// up and running.
	const (
		a = "recover-test-a"
		b = "recover-test-b"
	)
	// Need to explicitly call deleting these two namespaces because this function is called from the issues test
	// ("Should allow deletion of namespaces with propagated objects that can't be removed - issue #1214", which tries
	// to delete a specific namespace "child" . Cleaning up all namespaces using CleanupTestNamespaces will cause that
	// test to fail.
	cleanupNamespaces(a, b)
	// Ensure validators work
	mustRunWithTimeout(1, 30, "kubectl create ns", a)
	// Ensure reconcilers work
	mustRunWithTimeout(1, 30, "kubectl hns create", b, "-n", a)
	mustRunWithTimeout(1, 30, "kubectl get ns", b)
	cleanupNamespaces(a, b)
}

func writeTempFile(cxt string) string {
	f, err := ioutil.TempFile(os.TempDir(), "e2e-test-*.yaml")
	Expect(err).Should(BeNil())
	defer f.Close()
	f.WriteString(cxt)
	return f.Name()
}

func removeFile(path string) {
	Expect(os.Remove(path)).Should(BeNil())
}

// silencer is a matcher that assumes that an empty string is good, and any
// non-empty string means that test failed. You use it by saying
// `Should(beQuiet())` instead of `Should(Equal(""))`, which both looks
// moderately nicer in the code but more importantly produces much nicer error
// messages if it fails. You should never say `ShouldNot(beQuiet())`.
//
// See https://onsi.github.io/gomega/#adding-your-own-matchers for details.
type silencer struct{}
func beQuiet() silencer {return silencer{}}
func (_ silencer) Match(actual interface{}) (bool, error) {
	diffs := actual.(string)
	return diffs == "", nil
}
func (_ silencer) FailureMessage(actual interface{}) string {
	return actual.(string)
}
func (_ silencer) NegatedFailureMessage(actual interface{}) string {
	return "!!!! you should not put beQuiet() in a ShouldNot matcher !!!!"
}

