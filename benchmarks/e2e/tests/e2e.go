package test

import (
	"testing"

	"github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"
	"k8s.io/component-base/logs"
	ginkgowrapper "k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"

	// test sources
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/block_cluster_resources"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/block_host_pid"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/block_ns_quotas"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/block_other_tenant_resources"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/block_privileged_containers"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/configure_ns_quotas"
	_ "sigs.k8s.io/multi-tenancy/benchmarks/e2e/tests/create_role_bindings"
)

// RunE2ETests runs the multi-tenancy benchmark tests
func RunE2ETests(t *testing.T) {
	logs.InitLogs()
	defer logs.FlushLogs()

	gomega.RegisterFailHandler(ginkgowrapper.Fail)
	ginkgo.RunSpecs(t, "Multi-Tenancy Benchmarks")
}
