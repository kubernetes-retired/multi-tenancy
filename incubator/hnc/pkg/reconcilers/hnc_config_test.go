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
)

const (
	// nopTime is the time we're willing to wait to ensure that something _hasn't_ happened
	// ("no-operation time"). If we expect something *to* happen, then we use Eventually().
	//
	// From experiments it takes ~0.015s for HNC to propagate an object. Setting the sleep time to 1s
	// should be long enough.
	//
	// We may need to increase the sleep time in future if HNC takes longer to propagate objects.
	nopTime = 1 * time.Second

	// statusUpdateTime is the timeout for `Eventually` to verify the object counts in the HNC Config
	// status.  Currently the config reconciler periodically updates status every 3 seconds. From
	// experiments, tests are flaky when setting the statusUpdateTime to 3 seconds and tests can
	// always pass when setting the time to 4 seconds. We may need to increase the time in future if
	// the config reconciler takes longer to update the status.
	statusUpdateTime = 4 * time.Second

	// rbacAV is a nice short form of the RBAC APIVersion
	rbacAV = "rbac.authorization.k8s.io/v1"

	// testModeMisssing is a fake mode to indicate that the spec/status doesn't exist in the config
	testModeMisssing api.SynchronizationMode = "<missing>"
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

	It("should set mode of Roles and RoleBindings as propagate by default", func() {
		Eventually(typeSpecHasMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
		Eventually(typeSpecHasMode(ctx, rbacAV, "RoleBinding")).Should(Equal(api.Propagate))
	})

	It("should propagate Roles by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role.
		makeObject(ctx, "Role", fooName, "foo-role")

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
	})

	It("should insert Roles if it does not exist", func() {
		removeHNCConfigType(ctx, rbacAV, "Role")

		Eventually(typeSpecHasMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
	})

	It("should set the mode of Roles to `propagate` if the mode is not `propagate`", func() {
		updateHNCConfigSpec(ctx, rbacAV, "Role", api.Ignore)

		Eventually(typeSpecHasMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role binding.
		makeRoleBinding(ctx, fooName, "foo-role", "foo-admin", "foo-role-binding")

		Eventually(hasObject(ctx, "RoleBinding", barName, "foo-role-binding")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "RoleBinding", barName, "foo-role-binding")).Should(Equal(fooName))
	})

	It("should insert RoleBindings if it does not exist", func() {
		removeHNCConfigType(ctx, rbacAV, "RoleBinding")

		Eventually(typeSpecHasMode(ctx, rbacAV, "RoleBinding")).Should(Equal(api.Propagate))
	})

	It("should set the mode of RoleBindings to `propagate` if the mode is not `propagate`", func() {
		updateHNCConfigSpec(ctx, rbacAV, "RoleBinding", api.Ignore)

		Eventually(typeSpecHasMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
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

	It("should unset ObjectReconcilerCreationFailed condition if a bad type spec is removed", func() {
		// API version of ConfigMap should be "v1"
		addToHNCConfig(ctx, "v2", "ConfigMap", api.Propagate)

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=ConfigMap")).Should(BeTrue())

		removeHNCConfigType(ctx, "v2", "ConfigMap")

		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "/v2, Kind=ConfigMap")).Should(BeFalse())
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
		time.Sleep(nopTime)
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

	It("should stop propagating objects if the mode of a type is changed to ignore", func() {
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "Secret", fooName, "foo-sec")

		// Foo should have "foo-sec" since we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec")).Should(BeTrue())
		// "foo-sec" should now be propagated from foo to bar because we set the mode of Secret to "propagate".
		Eventually(hasObject(ctx, "Secret", barName, "foo-sec")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Secret", barName, "foo-sec")).Should(Equal(fooName))

		// Change to ignore and wait for reconciler
		updateHNCConfigSpec(ctx, "v1", "Secret", api.Ignore)
		Eventually(typeStatusHasMode(ctx, "v1", "Secret")).Should(Equal(api.Ignore))

		bazName := createNS(ctx, "baz")
		setParent(ctx, bazName, fooName)
		// Sleep to give "foo-sec" a chance to propagate from foo to baz, if it could.
		time.Sleep(nopTime)
		Expect(hasObject(ctx, "Secret", bazName, "foo-sec")()).Should(BeFalse())
	})

	It("should propagate objects if the mode of a type is changed from ignore to propagate", func() {
		addToHNCConfig(ctx, "v1", "ResourceQuota", api.Ignore)

		setParent(ctx, barName, fooName)
		makeObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")

		// Foo should have "foo-resource-quota" since we created there.
		Eventually(hasObject(ctx, "ResourceQuota", fooName, "foo-resource-quota")).Should(BeTrue())
		// Sleep to give "foo-resource-quota" a chance to propagate from foo to bar, if it could.
		time.Sleep(nopTime)
		Expect(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")()).Should(BeFalse())

		updateHNCConfigSpec(ctx, "v1", "ResourceQuota", api.Propagate)
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

		updateHNCConfigSpec(ctx, "v1", "Secret", api.Remove)

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
		time.Sleep(nopTime)
		// "foo-resource-quota" should not be propagated from foo to bar.
		Expect(hasObject(ctx, "ResourceQuota", barName, "foo-resource-quota")()).Should(BeFalse())

		updateHNCConfigSpec(ctx, "v1", "ResourceQuota", api.Propagate)

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

		// Remove from spec and wait for the reconciler to pick it up
		removeHNCConfigType(ctx, "v1", "Secret")
		Eventually(typeStatusHasMode(ctx, "v1", "Secret")).Should(Equal(testModeMisssing))

		// Give foo another secret.
		makeObject(ctx, "Secret", fooName, "foo-sec-2")
		// Foo should have "foo-sec-2" because we created there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec-2")).Should(BeTrue())
		// Sleep to give "foo-sec-2" a chance to propagate from foo to bar, if it could.
		time.Sleep(nopTime)
		// "foo-role-2" should not propagate from foo to bar because the reconciliation request is ignored.
		Expect(hasObject(ctx, "Secret", barName, "foo-sec-2")()).Should(BeFalse())

	})

	It("should reconcile after adding a new crd to the apiserver", func() {
		// Add a config for a type that hasn't been defined yet.
		addToHNCConfig(ctx, "stable.example.com/v1", "CronTab", api.Propagate)

		// The corresponding object reconciler should not be created because the type does not exist.
		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "stable.example.com/v1, Kind=CronTab")).Should(BeTrue())

		// Add the CRD for CronTab to the apiserver.
		createCronTabCRD(ctx)

		// The object reconciler for CronTab should be created successfully.
		Eventually(hasHNCConfigurationConditionWithMsg(ctx, api.ObjectReconcilerCreationFailed, "stable.example.com/v1, Kind=CronTab")).Should(BeFalse())

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

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(1))

		deleteObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(0))
	})

	It("should set NumPropagatedObjects back to 0 after switching from propagate to remove mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(1))

		updateHNCConfigSpec(ctx, "v1", "LimitRange", api.Remove)

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(0))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	It("should set NumSourceObjects for a type in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(1))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	// If a mode is unset, it is treated as `propagate` by default, in which case we will also compute NumSourceObjects
	It("should set NumSourceObjects for a type with unset mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", "")
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(1))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	It("should decrement NumSourceObjects correctly after deleting an object of a type in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(1))

		deleteObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(Equal(0))
	})

	It("should not set NumSourceObjects for a type not in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Remove)

		Eventually(hasNumSourceObjects(ctx, "v1", "LimitRange"), statusUpdateTime).Should(BeFalse())
	})
})

func typeSpecHasMode(ctx context.Context, apiVersion, kind string) func() api.SynchronizationMode {
	return func() api.SynchronizationMode {
		config := getHNCConfig(ctx)
		for _, t := range config.Spec.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				return t.Mode
			}
		}
		return testModeMisssing
	}
}

func typeStatusHasMode(ctx context.Context, apiVersion, kind string) func() api.SynchronizationMode {
	return func() api.SynchronizationMode {
		config := getHNCConfig(ctx)
		for _, t := range config.Status.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				return t.Mode
			}
		}
		return testModeMisssing
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

func updateHNCConfigSpec(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	Eventually(func() error {
		// Get the existing spec from the apiserver
		c := getHNCConfig(ctx)

		// Modify the existing spec. We should find the thing we were looking for.
		found := false
		for i := 0; i < len(c.Spec.Types); i++ { // don't use range-for since that creates copies of the objects
			if c.Spec.Types[i].APIVersion == apiVersion && c.Spec.Types[i].Kind == kind {
				c.Spec.Types[i].Mode = mode
				found = true
				break
			}
		}
		Expect(found).Should(BeTrue())

		// Update the apiserver
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
func getNumPropagatedObjects(ctx context.Context, apiVersion, kind string) func() (int, error) {
	return func() (int, error) {
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

// getNumSourceObjects returns NumSourceObjects status for a given type. If NumSourceObjects is
// not set or if type does not exist in status, it returns -1 and an error.
func getNumSourceObjects(ctx context.Context, apiVersion, kind string) func() (int, error) {
	return func() (int, error) {
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
