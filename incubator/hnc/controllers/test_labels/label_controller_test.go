package test_labels

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/controllers"
)

var _ = Describe("Labelled namespace", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")

		// Add a quick delay so that the controller is idle
		// TODO: look into why the controller is missing events.
		time.Sleep(100 * time.Millisecond)
	})

	It("should set a child on the parent", func() {
		Eventually(func() []string {
			foo := getNamespace(ctx, fooName)
			foo.ObjectMeta.Labels[controllers.LabelParent] = barName
			k8sClient.Update(ctx, foo)
			bar := getNamespace(ctx, barName)
			return strings.Split(bar.ObjectMeta.Annotations[controllers.AnnotChildren], ";")
		}).Should(Equal([]string{fooName}))
	})

	It("should set the parent to a bad value if the parent is deleted", func() {
		// Set up the parent-child relationship
		setParent(ctx, barName, fooName, true)

		// Delete the parent. We can't actually delete the namespace because the test env
		// doesn't run the builting Namespace controller, but we *can* mark it as deleted
		// and also delete the singleton, which should be enough to prevent it from being
		// recreated as well as force the reconciler to believe that the namespace is gone.
		fooNS := &corev1.Namespace{}
		fooNS.Name = fooName
		Expect(k8sClient.Delete(ctx, fooNS)).Should(Succeed())

		// Verify that the parent pointer is now set to a bad value
		Eventually(func() string {
			bar := getNamespace(ctx, barName)
			return bar.ObjectMeta.Labels[controllers.LabelParent]
		}).Should(Equal("missing.parent." + fooName))
	})

	It("should set a condition if a self-cycle is detected", func() {
		Eventually(func() string {
			foo := getNamespace(ctx, fooName)
			foo.ObjectMeta.Labels[controllers.LabelParent] = fooName
			k8sClient.Update(ctx, foo)
			foo = getNamespace(ctx, fooName)
			conds, _ := foo.ObjectMeta.Annotations[controllers.AnnotConds]
			return conds
		}).ShouldNot(Equal(""))
	})

	It("should set a condition if a cycle is detected", func() {
		// Set up initial hierarchy
		setParent(ctx, barName, fooName, true)

		// Break it
		setParent(ctx, fooName, barName, false)
		Eventually(func() string {
			foo := getNamespace(ctx, fooName)
			return foo.ObjectMeta.Annotations[controllers.AnnotConds]
		}).ShouldNot(Equal(""))
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

	return nm
}

func getNamespace(ctx context.Context, nm string) *corev1.Namespace {
	return getNamespaceWithOffset(1, ctx, nm)
}

func getNamespaceWithOffset(offset int, ctx context.Context, nm string) *corev1.Namespace {
	nnm := types.NamespacedName{Name: nm}
	inst := &corev1.Namespace{}
	ExpectWithOffset(offset+1, k8sClient.Get(ctx, nnm, inst)).Should(Succeed())
	if inst.ObjectMeta.Labels == nil {
		inst.ObjectMeta.Labels = map[string]string{}
	}
	if inst.ObjectMeta.Annotations == nil {
		inst.ObjectMeta.Annotations = map[string]string{}
	}
	return inst
}

func setParent(ctx context.Context, nm string, pnm string, wait bool) {
	var oldPNM string
	// There can be changes between getting and putting the namespace, which results in a
	// transient error, so try this a few times.
	Eventually(func() error {
		inst := getNamespace(ctx, nm)
		oldPNM, _ = inst.ObjectMeta.Labels[controllers.LabelParent]
		inst.ObjectMeta.Labels[controllers.LabelParent] = pnm
		return k8sClient.Update(ctx, inst)
	}).Should(Succeed())

	if !wait {
		return
	}

	if oldPNM != "" {
		EventuallyWithOffset(1, func() []string {
			p := getNamespaceWithOffset(1, ctx, oldPNM)
			return strings.Split(p.ObjectMeta.Annotations[controllers.AnnotChildren], ";")
		}).ShouldNot(ContainElement(nm))
	}
	if pnm != "" {
		EventuallyWithOffset(1, func() []string {
			p := getNamespaceWithOffset(1, ctx, pnm)
			return strings.Split(p.ObjectMeta.Annotations[controllers.AnnotChildren], ";")
		}).Should(ContainElement(nm))
	}
}
