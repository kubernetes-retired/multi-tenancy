package reconcilers_test

import (
	"context"
	"time"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("HNCConfiguration", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
	})

	AfterEach(func() {
		// Change current config back to the default value.
		Eventually(func() error {
			return resetHNCConfigToDefault(ctx)
		}).Should(Succeed())
	})

	It("should propagate objects whose types have been added to HNCConfiguration", func() {
		setParent(ctx, barName, fooName)
		makeSecret(ctx, fooName, "foo-sec")

		// Wait 1 second to give "foo-sec" a chance to be propagated to bar, if it can be propagated.
		time.Sleep(1 * time.Second)
		// Foo should have "foo-sec" since we created it there.
		Eventually(hasSecret(ctx, fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" is not propagated to bar because Secret hasn't been configured in HNCConfiguration.
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeFalse())

		Eventually(func() error {
			c := getHNCConfig(ctx)
			return addSecretToHNCConfig(ctx, c)
		}).Should(Succeed())

		// "foo-sec" should now be propagated from foo to bar.
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))
	})
})

func addSecretToHNCConfig(ctx context.Context, c *api.HNCConfiguration) error {
	secSpec := api.TypeSynchronizationSpec{APIVersion: "v1", Kind: "Secret", Mode: api.Propagate}
	c.Spec.Types = append(c.Spec.Types, secSpec)
	return updateHNCConfig(ctx, c)
}

func makeSecret(ctx context.Context, nsName, secretName string) {
	sec := &corev1.Secret{}
	sec.Name = secretName
	sec.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Create(ctx, sec)).Should(Succeed())
}

func hasSecret(ctx context.Context, nsName, secretName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
		sec := &corev1.Secret{}
		err := k8sClient.Get(ctx, nnm, sec)
		return err == nil
	}
}

func secretInheritedFrom(ctx context.Context, nsName, secretName string) string {
	nnm := types.NamespacedName{Namespace: nsName, Name: secretName}
	sec := &corev1.Secret{}
	if err := k8sClient.Get(ctx, nnm, sec); err != nil {
		// should have been caught above
		return err.Error()
	}
	if sec.ObjectMeta.Labels == nil {
		return ""
	}
	lif, _ := sec.ObjectMeta.Labels["hnc.x-k8s.io/inheritedFrom"]
	return lif
}
