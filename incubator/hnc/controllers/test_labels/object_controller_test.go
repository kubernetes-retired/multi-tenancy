package test_labels

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Secret", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
		bazName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
		bazName = createNS(ctx, "baz")

		// Start with the tree foo -> bar -> baz
		setParent(ctx, barName, fooName, true)
		setParent(ctx, bazName, barName, true)

		// Give them each a secret
		makeSecret(ctx, fooName, "foo-sec")
		makeSecret(ctx, barName, "bar-sec")
		makeSecret(ctx, bazName, "baz-sec")

		// Add a quick delay so that the controller is idle
		// TODO: look into why the controller is missing events.
		time.Sleep(100 * time.Millisecond)
	})

	It("should be copied to descendents", func() {
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
	})

	It("should be removed if the hierarchy changes", func() {
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeTrue())
		setParent(ctx, bazName, fooName, true)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeTrue())
		setParent(ctx, bazName, "", true)
		Eventually(hasSecret(ctx, bazName, "bar-sec")).Should(BeFalse())
		Eventually(hasSecret(ctx, bazName, "foo-sec")).Should(BeFalse())
	})
})

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
