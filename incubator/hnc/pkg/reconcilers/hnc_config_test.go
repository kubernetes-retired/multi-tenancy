package reconcilers_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("HNCConfiguration", func() {
	// objectPropagationTime is the time to sleep for objects propagation to take effect.
	// From experiment it takes ~0.015s for HNC to propagate an object. Setting
	// the sleep time to 1s should be long enough.
	// We may need to increase the sleep time in future if HNC takes longer to propagate objects.
	const objectPropagationTime = 1 * time.Second
	// hncConfigReconciliationTime is the time to wait for reconciliation of the config
	// object happens. It is the same as the sleep time at the beginning of the Reconcile method
	// of HNCConfiguration.
	const hncConfigReconciliationTime = 3 * time.Second
	// hncConfigStatusUpdateTime is the time to wait for updating HNCConfiguration status on
	// the apiserver. From experiment, this is the minimal integer seconds needed for the
	// status to be updated successfully on the apiserver.
	// We may need to increase the time in future if it takes longer to update status.
	const hncConfigStatusUpdateTime = 7 * time.Second
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		// Increase the timeout of `Eventually` for test cases in this file because the `Reconcile`
		// method of ConfigReconciler sleeps at the very beginning of the method.
		SetDefaultEventuallyTimeout(4 * time.Second)
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
	})

	AfterEach(func() {
		// Change current config back to the default value.
		Eventually(func() error {
			return resetHNCConfigToDefault(ctx)
		}).Should(Succeed())
		SetDefaultEventuallyTimeout(2 * time.Second)
	})

	It("should set mode of Roles and RoleBindings as propagate by default", func() {
		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "Role", api.Propagate)).Should(BeTrue())
		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "RoleBinding", api.Propagate)).Should(BeTrue())
	})

	It("should propagate Roles by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role.
		makeObject(ctx, "Role", fooName, "foo-role")

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
	})

	It("should insert Roles if it does not exist", func() {
		removeHNCConfigType(ctx, "rbac.authorization.k8s.io/v1", "Role")

		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "Role", api.Propagate)).Should(BeTrue())
	})

	It("should set the mode of Roles to `propagate` if the mode is not `propagate`", func() {
		updateHNCConfigSpec(ctx, "rbac.authorization.k8s.io/v1", "rbac.authorization.k8s.io/v1",
			"Role", "Role", api.Propagate, api.Ignore)

		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "Role", api.Propagate)).Should(BeTrue())
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role binding.
		makeRoleBinding(ctx, fooName, "foo-role", "foo-admin", "foo-role-binding")

		Eventually(hasObject(ctx, "RoleBinding", barName, "foo-role-binding")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "RoleBinding", barName, "foo-role-binding")).Should(Equal(fooName))
	})

	It("should insert RoleBindings if it does not exist", func() {
		removeHNCConfigType(ctx, "rbac.authorization.k8s.io/v1", "RoleBinding")

		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "RoleBinding", api.Propagate)).Should(BeTrue())
	})

	It("should set the mode of RoleBindings to `propagate` if the mode is not `propagate`", func() {
		updateHNCConfigSpec(ctx, "rbac.authorization.k8s.io/v1", "rbac.authorization.k8s.io/v1",
			"RoleBinding", "RoleBinding", api.Propagate, api.Ignore)

		Eventually(hasTypeWithMode(ctx, "rbac.authorization.k8s.io/v1", "Role", api.Propagate)).Should(BeTrue())
	})

	It("should set CritSingletonNameInvalid condition if singleton name is wrong", func() {
		nm := "wrong-config-1"
		Eventually(func() error {
			config := &api.HNCConfiguration{}
			config.ObjectMeta.Name = nm
			return updateHNCConfig(ctx, config)
		}).Should(Succeed())

		Eventually(hasHNCConfigurationConditionWithName(ctx, api.CritSingletonNameInvalid, nm), hncConfigStatusUpdateTime).Should(BeTrue())
	})

	It("should unset ObjectReconcilerCreationFailed condition if an object reconciler creation later succeeds", func() {
		// API version of ConfigMap should be "v1"
		addToHNCConfig(ctx, "v2", "ConfigMap", api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=ConfigMap"), hncConfigStatusUpdateTime).Should(BeTrue())

		updateHNCConfigSpec(ctx, "v2", "v1", "ConfigMap", "ConfigMap", api.Propagate, api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=ConfigMap"), hncConfigStatusUpdateTime).Should(BeFalse())
	})

	It("should set MultipleConfigurationsForOneType if there are multiple configurations for one type", func() {
		// Add multiple configurations for a type.
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		addToHNCConfig(ctx, "v1", "Secret", api.Remove)

		// The second configuration should be ignored.
		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.MultipleConfigurationsForOneType,
			fmt.Sprintf("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))).Should(BeTrue())
	})

	It("should unset MultipleConfigurationsForOneType if extra configurations are later removed", func() {
		// Add multiple configurations for a type.
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		addToHNCConfig(ctx, "v1", "Secret", api.Remove)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.MultipleConfigurationsForOneType,
			fmt.Sprintf("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))).Should(BeTrue())

		removeHNCConfigTypeWithMode(ctx, "v1", "Secret", api.Remove)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.MultipleConfigurationsForOneType,
			fmt.Sprintf("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))).Should(BeFalse())
	})

	It("should not propagate objects if the type is not in HNCConfiguration", func() {
		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" since we created there.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(objectPropagationTime)
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
		time.Sleep(hncConfigReconciliationTime)

		bazName := createNS(ctx, "baz")
		setParent(ctx, bazName, fooName)
		// Sleep to give "foo-sec" a chance to propagate from foo to baz, if it could.
		time.Sleep(objectPropagationTime)
		Expect(hasObject(ctx, "Secret", bazName, "foo-sec")()).Should(BeFalse())
	})

	It("should propagate objects if the mode of a type is changed from ignore to propagate", func() {
		addToHNCConfig(ctx, "v1", "ResourceQuota", api.Ignore)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" since we created there.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(objectPropagationTime)
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
		time.Sleep(objectPropagationTime)
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
		time.Sleep(hncConfigReconciliationTime)
		// Give foo another secret.
		makeObject(ctx, "Secret", fooName, "foo-sec-2")

		// Foo should have "foo-sec-2" because we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec-2")).Should(BeTrue())
		// Sleep to give "foo-sec-2" a chance to propagate from foo to bar, if it could.
		time.Sleep(objectPropagationTime)
		// "foo-role-2" should not propagate from foo to bar because the reconciliation request is ignored.
		Expect(hasObject(ctx, "Secret", barName, "foo-sec-2")()).Should(BeFalse())

	})

	It("should reconcile after adding a new crd to the apiserver", func() {
		// Add a config for a type that hasn't been defined yet.
		addToHNCConfig(ctx, "stable.example.com/v1", "CronTab", api.Propagate)

		// The corresponding object reconciler should not be created because the type does not exist.
		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed,
			"stable.example.com/v1, Kind=CronTab"), hncConfigStatusUpdateTime).Should(BeTrue())

		// Add the CRD for CronTab to the apiserver.
		createCronTabCRD(ctx)

		// The object reconciler for CronTab should be created successfully.
		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed,
			"stable.example.com/v1, Kind=CronTab"), hncConfigStatusUpdateTime).Should(BeFalse())

		// Give foo a CronTab object.
		setParent(ctx, barName, fooName)
		makeObject(ctx, "CronTab", fooName, "foo-crontab")

		// "foo-crontab" should be propagated from foo to bar.
		Eventually(hasObject(ctx, "CronTab", barName, "foo-crontab")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "CronTab", barName, "foo-crontab")).Should(Equal(fooName))
	})

	It("should set NumPropagatedObjects back to 0 after deleting the source object in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(1)))

		deleteObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(0)))
	})

	It("should set NumPropagatedObjects back to 0 after switching from propagate to remove mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(1)))

		updateHNCConfigSpec(ctx, "v1", "v1", "LimitRange", "LimitRange", api.Propagate, api.Remove)

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(0)))
	})

	It("should set NumSourceObjects for a type in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		// There should be 2 source objects of LimitRange: one added in this test case, the other added in the previous test case.
		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(2)))
	})

	// If a mode is unset, it is treated as `propagate` by default, in which case we will also compute NumSourceObjects
	It("should set NumSourceObjects for a type with unset mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", "")
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		// There should be 3 source objects of LimitRange: one added in this test case, the other 2 added in the previous two test cases.
		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(Equal(int32(3)))
	})

	It("should not set NumSourceObjects for a type not in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Remove)

		Eventually(hasNumSourceObjects(ctx, "v1", "LimitRange"), hncConfigStatusUpdateTime).Should(BeFalse())
	})
})

func hasTypeWithMode(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) func() bool {
	// `Eventually` only works with a fn that doesn't take any args
	return func() bool {
		config := getHNCConfig(ctx)
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

func removeHNCConfigTypeWithMode(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	Eventually(func() error {
		c := getHNCConfig(ctx)
		i := 0
		for ; i < len(c.Spec.Types); i++ {
			if c.Spec.Types[i].APIVersion == apiVersion && c.Spec.Types[i].Kind == kind && c.Spec.Types[i].Mode == mode {
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

func createCronTabCRD(ctx context.Context) {
	crontab := v1beta1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "crontabs.stable.example.com",
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group: "stable.example.com",
			Versions: []v1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
			Names: v1beta1.CustomResourceDefinitionNames{
				Singular: "crontab",
				Plural:   "crontabs",
				Kind:     "CronTab",
			},
		},
	}
	Eventually(func() error {
		return k8sClient.Create(ctx, &crontab)
	}).Should(Succeed())
}

// getNumPropagatedObjects returns NumPropagatedObjects status for a given type. If NumPropagatedObjects is
// not set or if type does not exist in status, it returns -1 and an error.
func getNumPropagatedObjects(ctx context.Context, apiVersion, kind string) func() (int32, error) {
	return func() (int32, error) {
		c := getHNCConfig(ctx)
		for _, t := range c.Status.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				if t.NumPropagatedObjects != nil {
					return *t.NumPropagatedObjects, nil
				}
				return -1, errors.New(fmt.Sprintf("NumPropagatedObjects field is not set for "+
					"apiversion %s, kind %s", apiVersion, kind))
			}
		}
		return -1, errors.New(fmt.Sprintf("apiversion %s, kind %s is not found in status", apiVersion, kind))
	}
}

// hasNumSourceObjects returns true if NumSourceObjects is set (not nil) for a specific type and returns false
// if NumSourceObjects is not set. It returns false and an error if the type does not exist in the status.
func hasNumSourceObjects(ctx context.Context, apiVersion, kind string) func() (bool, error) {
	return func() (bool, error) {
		c := getHNCConfig(ctx)
		for _, t := range c.Status.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				return t.NumSourceObjects != nil, nil
			}
		}
		return false, errors.New(fmt.Sprintf("apiversion %s, kind %s is not found in status", apiVersion, kind))
	}
}

// getNumPropagatedObjects returns NumSourceObjects status for a given type. If NumSourceObjects is
// not set or if type does not exist in status, it returns -1 and an error.
func getNumSourceObjects(ctx context.Context, apiVersion, kind string) func() (int32, error) {
	return func() (int32, error) {
		c := getHNCConfig(ctx)
		for _, t := range c.Status.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				if t.NumSourceObjects != nil {
					return *t.NumSourceObjects, nil
				}
				return -1, errors.New(fmt.Sprintf("NumSourceObjects field is not set for "+
					"apiversion %s, kind %s", apiVersion, kind))
			}
		}
		return -1, errors.New(fmt.Sprintf("apiversion %s, kind %s is not found in status", apiVersion, kind))
	}
}

// deleteObject deletes an object of the given kind in a specific namespace. The kind and
// its corresponding GVK should be included in the GVKs map.
func deleteObject(ctx context.Context, kind string, nsName, name string) {
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(GVKs[kind])
	inst.SetNamespace(nsName)
	inst.SetName(name)
	ExpectWithOffset(1, k8sClient.Delete(ctx, inst)).Should(Succeed())
}
