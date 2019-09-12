package controllers_test

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("Namespaces", func() {
	ctx := context.Background()

	var fooName string
	BeforeEach(func() {
		fooName = createNSName("foo")

		// Add a quick delay so that the controller is idle
		// TODO: look into why the controller is missing events.
		time.Sleep(100 * time.Millisecond)
	})

	It("should create a hier object", func() {
		// Create the namespace
		ns := &corev1.Namespace{}
		ns.Name = fooName
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		// Wait for the singleton to appear
		snm := types.NamespacedName{Namespace: fooName, Name: tenancy.Singleton}
		hier := &tenancy.Hierarchy{}
		Eventually(func() error {
			return k8sClient.Get(ctx, snm, hier)
		}).Should(Succeed())
	})
})

// createNSName generates random namespace names. Namespaces are never deleted in test-env because
// the building Namespace controller (which finalizes namespaces) doesn't run; I searched Github and
// found that everyone who was deleting namespaces was *also* very intentionally generating random
// names, so I guess this problem is widespread.
func createNSName(prefix string) string {
	suffix := make([]byte, 10)
	rand.Read(suffix)
	return fmt.Sprintf("%s-%x", prefix, suffix)
}

// createNS is a convenience function to create a namespace and wait for its singleton to be
// created. It's used in other tests in this package, but basically duplicates the code in this test
// (it didn't originally). TODO: refactor.
func createNS(ctx context.Context, prefix string) string {
	nm := createNSName(prefix)

	// Create the namespace
	ns := &corev1.Namespace{}
	ns.Name = nm
	Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

	// Wait for the Hierarchy singleton to be created
	snm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	hier := &tenancy.Hierarchy{}
	Eventually(func() error {
		return k8sClient.Get(ctx, snm, hier)
	}).Should(Succeed())

	return nm
}
