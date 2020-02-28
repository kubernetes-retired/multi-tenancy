package reconcilers_test

import (
	"context"
	"strings"

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

		Eventually(hasRole(ctx, barName, "foo-role")).Should(BeTrue())
		Expect(roleInheritedFrom(ctx, barName, "foo-role")).Should(Equal(fooName))
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)

		Eventually(hasRoleBinding(ctx, barName, "foo-role-binding")).Should(BeTrue())
		Expect(roleBindingInheritedFrom(ctx, barName, "foo-role-binding")).Should(Equal(fooName))
	})

	It("should set CritSingletonNameInvalid condition if singleton name is wrong", func() {
		nm := "wrong-config-1"
		Eventually(func() error {
			config := &api.HNCConfiguration{}
			config.ObjectMeta.Name = nm
			return updateHNCConfig(ctx, config)
		}).Should(Succeed())

		Eventually(hasHNCConfigurationConditionWithName(ctx, api.CritSingletonNameInvalid, nm)).Should(BeTrue())
	})

	It("should set ObjectReconcilerCreationFailed condition if an object reconciler creation fails", func() {
		// API version of Secret should be "v1"
		addToHNCConfig(ctx, "v2", "ConfigMap", api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=ConfigMap")).Should(BeTrue())
	})

	It("should unset ObjectReconcilerCreationFailed condition if an object reconciler creation later succeeds", func() {
		// API version of LimitRange should be "v1"
		addToHNCConfig(ctx, "v2", "LimitRange", api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=LimitRange")).Should(BeTrue())

		updateHNCConfigSpec(ctx, "v2", "v1", "LimitRange", "LimitRange", api.Propagate, api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=LimitRange")).Should(BeFalse())
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

func hasHNCConfigurationConditionWithMsg(ctx context.Context, code api.HNCConfigurationCode, subMsg string) func() bool {
	return func() bool {
		c := getHNCConfig(ctx)
		return hasHNCConfigurationConditionWithSingletonAndSubMsg(code, subMsg, c)
	}
}

func hasHNCConfigurationConditionWithName(ctx context.Context, code api.HNCConfigurationCode, nm string) func() bool {
	return func() bool {
		c := getHNCConfigWithOffsetAndName(1, ctx, nm)
		// Use an empty string here to match Msg field so that the match always succeeds.
		// It is not necessary and less robust to check the error message since the
		// name invalidation error is not associated with a specific type.
		return hasHNCConfigurationConditionWithSingletonAndSubMsg(code, "", c)
	}
}

func hasHNCConfigurationConditionWithSingletonAndSubMsg(code api.HNCConfigurationCode, subMsg string, c *api.HNCConfiguration) bool {
	conds := c.Status.Conditions
	if code == "" {
		return len(conds) > 0
	}
	for _, cond := range conds {
		if cond.Code == code && strings.Contains(cond.Msg, subMsg) {
			return true
		}
	}
	return false
}

func updateHNCConfigSpec(ctx context.Context, prevApiVersion, newApiVersion, prevKind, newKind string,
	preMode, newMode api.SynchronizationMode) {
	Eventually(func() error {
		c := getHNCConfig(ctx)
		for i := 0; i < len(c.Spec.Types); i++ {
			if c.Spec.Types[i].APIVersion == prevApiVersion && c.Spec.Types[i].Kind == prevKind && c.Spec.Types[i].Mode == preMode {
				c.Spec.Types[i].APIVersion = newApiVersion
				c.Spec.Types[i].Kind = newKind
				c.Spec.Types[i].Mode = newMode
				break
			}
		}
		return updateHNCConfig(ctx, c)
	}).Should(Succeed())
}
