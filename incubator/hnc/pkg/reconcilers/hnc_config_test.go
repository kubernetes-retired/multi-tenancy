package reconcilers_test

import (
	"context"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		// Give foo a role.
		makeRole(ctx, fooName, "foo-role")
		// Give foo a role binding.
		makeRoleBinding(ctx, fooName, "foo-role", "foo-admin", "foo-role-binding")
	})

	AfterEach(func() {
		// Change current config back to the default value.
		Eventually(func() error {
			return resetHNCConfigToDefault(ctx)
		}).Should(Succeed())
	})

	It("should set mode of Roles and RoleBindings as propagate by default", func() {
		config := getHNCConfig(ctx)

		Eventually(hasTypeWithMode("rbac.authorization.k8s.io/v1", "Role", api.Propagate, config)).Should(BeTrue())
		Eventually(hasTypeWithMode("rbac.authorization.k8s.io/v1", "RoleBinding", api.Propagate, config)).Should(BeTrue())
	})

	It("should propagate Roles by default", func() {
		setParent(ctx, barName, fooName)
		makeSecret(ctx, fooName, "foo-sec")

		Eventually(hasRole(ctx, barName, "foo-role")).Should(BeTrue())
		Expect(roleInheritedFrom(ctx, barName, "foo-role")).Should(Equal(fooName))
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)
		Eventually(hasRoleBinding(ctx, barName, "foo-role-binding")).Should(BeTrue())
		Expect(roleBindingInheritedFrom(ctx, barName, "foo-role-binding")).Should(Equal(fooName))
	})

	It("should only propagate objects whose types are in HNCConfiguration", func() {
		setParent(ctx, barName, fooName)
		makeSecret(ctx, fooName, "foo-sec")
		makeResourceQuota(ctx, fooName, "foo-resource-quota")

		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)

		// Foo should have both "foo-sec" and "foo-resource-quota" since we created there.
		Eventually(hasSecret(ctx, fooName, "foo-sec")).Should(BeTrue())
		Eventually(hasResourceQuota(ctx, fooName, "foo-resource-quota")).Should(BeTrue())
		// "foo-sec" should now be propagated from foo to bar.
		Eventually(hasSecret(ctx, barName, "foo-sec")).Should(BeTrue())
		Expect(secretInheritedFrom(ctx, barName, "foo-sec")).Should(Equal(fooName))
		// "foo-resource-quota" should not be propagated from foo to bar because ResourceQuota
		// is not added to HNCConfiguration.
		Expect(hasResourceQuota(ctx, barName, "foo-resource-quota")).Should(BeFalse())
	})
})

func hasTypeWithMode(apiVersion, kind string, mode api.SynchronizationMode, config *api.HNCConfiguration) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		for _, t := range config.Spec.Types {
			if t.APIVersion == apiVersion && t.Kind == kind && t.Mode == mode {
				return true
			}
		}
		return false
	}
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

func makeRoleBinding(ctx context.Context, nsName, roleName, userName, roleBindingName string) {
	roleBinding := &v1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: nsName,
		},
		Subjects: []v1.Subject{
			{
				Kind: "User",
				Name: userName,
			},
		},
		RoleRef: v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, roleBinding)).Should(Succeed())
}

func hasRoleBinding(ctx context.Context, nsName, roleBindingName string) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		nnm := types.NamespacedName{Namespace: nsName, Name: roleBindingName}
		roleBinding := &v1.RoleBinding{}
		err := k8sClient.Get(ctx, nnm, roleBinding)
		return err == nil
	}
}

func roleBindingInheritedFrom(ctx context.Context, nsName, roleBindingName string) string {
	nnm := types.NamespacedName{Namespace: nsName, Name: roleBindingName}
	roleBinding := &v1.RoleBinding{}
	if err := k8sClient.Get(ctx, nnm, roleBinding); err != nil {
		// should have been caught above
		return err.Error()
	}
	if roleBinding.ObjectMeta.Labels == nil {
		return ""
	}
	lif, _ := roleBinding.ObjectMeta.Labels["hnc.x-k8s.io/inheritedFrom"]
	return lif
}

func makeResourceQuota(ctx context.Context, nsName, resourceQuotaName string) {
	resourceQuota := &corev1.ResourceQuota{}
	resourceQuota.Name = resourceQuotaName
	resourceQuota.Namespace = nsName
	ExpectWithOffset(1, k8sClient.Create(ctx, resourceQuota)).Should(Succeed())
}

func hasResourceQuota(ctx context.Context, nsName, resourceQuotaName string) bool {
	// `Eventually` only works with a fn that doesn't take any args
	nnm := types.NamespacedName{Namespace: nsName, Name: resourceQuotaName}
	resourceQuota := &corev1.ResourceQuota{}
	err := k8sClient.Get(ctx, nnm, resourceQuota)
	return err == nil
}
