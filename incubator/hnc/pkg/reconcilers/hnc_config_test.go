package reconcilers_test

import (
	"context"
	"strings"
	"time"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("HNCConfiguration", func() {
	// sleepTime is the time to sleep for objects propagation to take effect.
	// From experiment it takes ~0.015s for HNC to propagate an object. Setting
	// the sleep time to 1s should be long enough.
	// We may need to increase the sleep time in future if HNC takes longer to propagate objects.
	const sleepTime = 1 * time.Second
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

	It("should set mode of Roles and RoleBindings as propagate by default", func() {
		config := getHNCConfig(ctx)

		Eventually(hasTypeWithMode("rbac.authorization.k8s.io/v1", "Role", api.Propagate, config)).Should(BeTrue())
		Eventually(hasTypeWithMode("rbac.authorization.k8s.io/v1", "RoleBinding", api.Propagate, config)).Should(BeTrue())
	})

	It("should propagate Roles by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role.
		makeObject(ctx, "Role", fooName, "foo-role")

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role binding.
		makeRoleBinding(ctx, fooName, "foo-role", "foo-admin", "foo-role-binding")

		Eventually(hasObject(ctx, "RoleBinding", barName, "foo-role-binding")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "RoleBinding", barName, "foo-role-binding")).Should(Equal(fooName))
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

	It("should not propagate objects if the type is not in HNCConfiguration", func() {
		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" since we created there.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(sleepTime)
		Expect(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")()).Should(BeFalse())
	})

	It("should propagate objects if the mode of a type is set to propagate", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", fooName, "foo-sec")

		// Foo should have "foo-sec" since we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" should now be propagated from foo to bar.
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Secret", barName, "foo-sec")).Should(Equal(fooName))
	})

	It("should stop propagating objects if the mode of a type is changed from propagate to ignore", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", fooName, "foo-sec")

		// Foo should have "foo-sec" since we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" should now be propagated from foo to bar because we set the mode of Secret to "propagate".
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Secret", barName, "foo-sec")).Should(Equal(fooName))

		updateHNCConfigSpec(ctx, "v1", "v1", "Secret", "Secret", api.Propagate, api.Ignore)
		bazName := createNS(ctx, "baz")
		setParent(ctx, bazName, fooName)
		// Sleep to give "foo-sec" a chance to propagate from foo to baz, if it could.
		time.Sleep(sleepTime)
		Expect(hasObject(ctx, "Secret", bazName, "foo-sec")()).Should(BeFalse())
	})

	It("should propagate objects if the mode of a type is changed from ignore to propagate", func() {
		addToHNCConfig(ctx, "v1", "ResourceQuota", api.Ignore)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" since we created there.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(sleepTime)
		Expect(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")()).Should(BeFalse())

		updateHNCConfigSpec(ctx, "v1", "v1", "ResourceQuota", "ResourceQuota", api.Ignore, api.Propagate)
		// "foo-resource-quota" should now be propagated from foo to bar because the mode of ResourceQuota is set to "propagate".
		Eventually(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "ResourceQuota", barName, "foo-resource-quota")).Should(Equal(fooName))
	})

	It("should remove propagated objects if the mode of a type is changed from propagate to remove", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", fooName, "foo-sec")

		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" should be propagated from foo to bar.
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Secret", barName, "foo-sec")).Should(Equal(fooName))

		updateHNCConfigSpec(ctx, "v1", "v1", "Secret", "Secret", api.Propagate, api.Remove)

		// Foo should still have "foo-sec" because it is a source object, not propagated one.
		// Therefore, we do not remove it.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" should be removed from bar.
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeFalse())
	})

	It("should propagate objects if the mode of a type is changed from remove to propagate", func() {
		addToHNCConfig(ctx, "v1", "ResourceQuota", api.Remove)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" because it is a source object, which will not be removed.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(sleepTime)
		// "foo-resource-quota" should not be propagated from foo to bar.
		Expect(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")()).Should(BeFalse())

		updateHNCConfigSpec(ctx, "v1", "v1", "ResourceQuota", "ResourceQuota", api.Remove, api.Propagate)

		// "foo-resource-quota" should be propagated from foo to bar.
		Eventually(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "ResourceQuota", barName, "foo-resource-quota")).Should(Equal(fooName))
	})

	It("should stop propagating objects if a type is first set to propagate mode then removed from the spec", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", fooName, "foo-sec")

		// "foo-sec" should propagate from foo to bar.
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Secret", barName, "foo-sec")).Should(Equal(fooName))

		removeHNCConfigType(ctx, "v1", "Secret")
		// Give foo another secret.
		makeObject(ctx, "Secret", fooName, "foo-sec-2")

		// Foo should have "foo-sec-2" because we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec-2")).Should(BeTrue())
		// Sleep to give "foo-sec-2" a chance to propagate from foo to bar, if it could.
		time.Sleep(sleepTime)
		// "foo-role-2" should not propagate from foo to bar because the reconciliation request is ignored.
		Expect(hasObject(ctx, "Secret", barName, "foo-sec-2")()).Should(BeFalse())

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

// We cannot use `makeObject` to create Rolebinding objects because `RoleRef` is a required field.
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

func removeHNCConfigType(ctx context.Context, apiVersion, kind string) {
	Eventually(func() error {
		c := getHNCConfig(ctx)
		i := 0
		for ; i < len(c.Spec.Types); i++ {
			if c.Spec.Types[i].APIVersion == apiVersion && c.Spec.Types[i].Kind == kind {
				break
			}
		}
		// The type does not exist. Nothing to remove.
		if i == len(c.Spec.Types) {
			return nil
		}
		c.Spec.Types[i] = c.Spec.Types[len(c.Spec.Types)-1]
		c.Spec.Types = c.Spec.Types[:len(c.Spec.Types)-1]
		return updateHNCConfig(ctx, c)
	}).Should(Succeed())
}
