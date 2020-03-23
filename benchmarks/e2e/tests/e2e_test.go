package test

import (
	"flag"
	"testing"

	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
}

func init() {
	// Register framework flags, then handle flags.
	handleFlags()
}

func TestE2E(t *testing.T) {
	flag.Parse()
	RunE2ETests(t)
}
