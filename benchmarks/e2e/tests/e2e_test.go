package test

import (
	"flag"
	"testing"

	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.StringVar(&configutil.ConfigPath,"config", "../../config.yaml",
		"Path of the config file for the tests")
}

func init() {
	// Register framework flags, then handle flags.
	handleFlags()
}

func TestE2E(t *testing.T) {
	flag.Parse()
	RunE2ETests(t)
}
