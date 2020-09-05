package e2e

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

const (
	namspacePrefix = "e2e-test-"
	// A 1s timeout was too short; 2s *seems* stable and also matches the Ginkgo default
	defTimeout = 2
	// For the operations that involves propogation, 5s seems to be a more stable time choice
	propogationTimeout = 5
	// For the operations that involves deletion, 10s seems to be a more stable time
	cleanupTimeout = 10
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecsWithDefaultAndCustomReporters(t,
		"HNC Suite",
		[]Reporter{printer.NewlineReporter{}})
}
