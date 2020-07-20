package reporter

import (
	"context"
	"fmt"
	"log"

	v1alpha1 "github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
)

// PolicyReporter creates the policyreport object
type PolicyReporter struct {
	policy        v1alpha1.PolicyReport
	testSummaries []*TestSummary
}

// NewPolicyReporter returns the pointer of PolicyReporter
func NewPolicyReporter() *PolicyReporter {
	policyName := "policy-" + string(uuid.NewUUID())
	return &PolicyReporter{
		policy: v1alpha1.PolicyReport{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PolicyReport",
				APIVersion: "policy.kubernetes.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: policyName,
			},
		},
	}
}

// SuiteWillBegin prints banner and total benchmarks to be run
func (p *PolicyReporter) SuiteWillBegin(suiteSummary *SuiteSummary) {

}

// TestWillRun prints each test status
func (p *PolicyReporter) TestWillRun(testSummary *TestSummary) {
	var status v1alpha1.PolicyStatus
	if testSummary.Validation && testSummary.Test {
		status = "Pass"
	} else if testSummary.Validation && !testSummary.Test {
		status = "Fail"
	} else if !testSummary.Validation {
		status = "Error"
	} else {
		status = "Skip"
	}
	p.policy.Results = append(p.policy.Results, &v1alpha1.PolicyReportResult{
		Policy: testSummary.Benchmark.Title,
		Status: status,
		Scored: true,
	})
}

// SuiteDidEnd prints end result summary of benchmark suite
func (p *PolicyReporter) SuiteDidEnd(suiteSummary *SuiteSummary) {
	p.policy.Summary = v1alpha1.PolicyReportSummary{
		Pass:  suiteSummary.NumberOfPassedTests,
		Fail:  suiteSummary.NumberOfFailedTests,
		Skip:  suiteSummary.NumberOfSkippedTests,
		Error: suiteSummary.NumberOfFailedValidations,
	}

	CreatePolicy(suiteSummary.TenantAdminNamespace, p.policy)
}

func (p *PolicyReporter) FullSummary(finalSummary *FinalSummary) {

}

// CreatePolicy creates the policy object
func CreatePolicy(namespace string, policy v1alpha1.PolicyReport) {
	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		errExit("", err)
	}

	v1alpha1.AddToScheme(scheme.Scheme)

	crdConfig := *config
	crdConfig.ContentConfig.GroupVersion = &v1alpha1.GroupVersion
	crdConfig.APIPath = "/apis"
	crdConfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	crdConfig.UserAgent = rest.DefaultKubernetesUserAgent()

	c, err := rest.RESTClientFor(&crdConfig)
	if err != nil {
		panic(err)
	}

	result := v1alpha1.PolicyReport{}
	err = c.
		Post().
		Namespace(namespace).
		Resource("policyreports").
		Body(&policy).
		Do(context.TODO()).
		Into(&result)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println(result.ObjectMeta.Name, "is Created")
	}
}

func errExit(msg string, err error) {
	if err != nil {
		log.Fatalf("%s: %#v", msg, err)
	}
}
