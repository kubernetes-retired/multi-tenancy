package test

import (
	"testing"

	"github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"k8s.io/component-base/logs"
	ginkgowrapper "k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"

	// test sources
	_ "github.com/realshuting/multi-tenancy/benchmarks/e2e/tests/block_other_tenant_resources"
	_ "github.com/realshuting/multi-tenancy/benchmarks/e2e/tests/block_cluster_resources"
	_ "github.com/realshuting/multi-tenancy/benchmarks/e2e/tests/configure_ns_quotas"
)

// RunE2ETests runs the multi-tenancy benchmark tests
func RunE2ETests(t *testing.T) {
	logs.InitLogs()
	defer logs.FlushLogs()

	gomega.RegisterFailHandler(ginkgowrapper.Fail)
	ginkgo.RunSpecs(t, "Multi-Tenancy Benchmarks")
}
