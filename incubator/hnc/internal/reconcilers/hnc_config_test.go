package reconcilers_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
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

	// countUpdateTime is the timeout for `Eventually` to verify the object counts in the HNC Config
	// status.  Currently the config reconciler periodically updates status every 3 seconds. From
	// experiments on workstations, tests are flaky when setting the countUpdateTime to 3 seconds and
	// tests can always pass when setting the time to 4 seconds. We may need to increase the time in
	// future if the config reconciler takes longer to update the status.  This issue is logged at
	// https://github.com/kubernetes-sigs/multi-tenancy/issues/871
	//
	// Update: since Prow machines appear to be overloaded, and since we've seen some random failures
	// in counting tests, I'm increasing this to 6s - aludwin, Oct 2020
	countUpdateTime = 6 * time.Second

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
		resetHNCConfigToDefault(ctx)
		// We want to ensure we're working with a clean slate, in case a previous tests objects still exist
		cleanupObjects(ctx)

		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
	})

	AfterEach(func() {
		// Change current config back to the default value.
		resetHNCConfigToDefault(ctx)
		cleanupObjects(ctx)
	})

	It("should set mode of Roles and RoleBindings as propagate by default", func() {
		Eventually(typeSpecMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
		Eventually(typeSpecMode(ctx, rbacAV, "RoleBinding")).Should(Equal(api.Propagate))
	})

	It("should propagate Roles by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role.
		makeObject(ctx, "Role", fooName, "foo-role")

		Eventually(hasObject(ctx, "Role", barName, "foo-role")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "Role", barName, "foo-role")).Should(Equal(fooName))
	})

	It("should insert Roles if it does not exist", func() {
		removeTypeConfig(ctx, rbacAV, "Role")

		Eventually(typeSpecMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
	})

	It("should set the mode of Roles to `propagate` if the mode is not `propagate`", func() {
		updateTypeConfig(ctx, rbacAV, "Role", api.Ignore)

		Eventually(typeSpecMode(ctx, rbacAV, "Role")).Should(Equal(api.Propagate))
	})

	It("should propagate RoleBindings by default", func() {
		setParent(ctx, barName, fooName)
		// Give foo a role binding.
		makeRoleBinding(ctx, fooName, "foo-role", "foo-admin", "foo-role-binding")

		Eventually(hasObject(ctx, "RoleBinding", barName, "foo-role-binding")).Should(BeTrue())
		Expect(objectInheritedFrom(ctx, "RoleBinding", barName, "foo-role-binding")).Should(Equal(fooName))
	})

	It("should insert RoleBindings if it does not exist", func() {
		removeTypeConfig(ctx, rbacAV, "RoleBinding")

		Eventually(typeSpecMode(ctx, rbacAV, "RoleBinding")).Should(Equal(api.Propagate))
	})

	It("should set the mode of RoleBindings to `propagate` if the mode is not `propagate`", func() {
		updateTypeConfig(ctx, rbacAV, "RoleBinding", api.Ignore)

		Eventually(typeSpecMode(ctx, rbacAV, "RoleBinding")).Should(Equal(api.Propagate))
	})

	It("should unset ObjectReconcilerCreationFailed condition if a bad type spec is removed", func() {
		// API version of ConfigMap should be "v1"
		addToHNCConfig(ctx, "v2", "ConfigMap", api.Propagate)

		Eventually(getHNCConfigCondition(ctx, api.ObjectReconcilerCreationFailed)).Should(ContainSubstring("/v2, Kind=ConfigMap"))

		removeTypeConfig(ctx, "v2", "ConfigMap")

		Eventually(getHNCConfigCondition(ctx, api.ObjectReconcilerCreationFailed)).Should(Equal(""))
	})

	It("should set MultipleConfigurationsForOneType if there are multiple configurations for one type", func() {
		// Add multiple configurations for a type.
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		addToHNCConfig(ctx, "v1", "Secret", api.Remove)

		// The second configuration should be identified
		Eventually(getHNCConfigCondition(ctx, api.MultipleConfigurationsForOneType)).
			Should(ContainSubstring("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))
	})

	It("should unset MultipleConfigurationsForOneType if extra configurations are later removed", func() {
		// Add multiple configurations for a type.
		addToHNCConfig(ctx, "v1", "Secret", api.Propagate)
		addToHNCConfig(ctx, "v1", "Secret", api.Remove)

		Eventually(getHNCConfigCondition(ctx, api.MultipleConfigurationsForOneType)).
			Should(ContainSubstring("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))

		removeTypeConfigWithMode(ctx, "v1", "Secret", api.Remove)

		Eventually(getHNCConfigCondition(ctx, api.MultipleConfigurationsForOneType)).
			ShouldNot(ContainSubstring("APIVersion: v1, Kind: Secret, Mode: %s", api.Remove))
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
		setTypeConfig(ctx, "v1", "Secret", api.Ignore)

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

		setTypeConfig(ctx, "v1", "ResourceQuota", api.Propagate)
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

		setTypeConfig(ctx, "v1", "Secret", api.Remove)

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

		setTypeConfig(ctx, "v1", "ResourceQuota", api.Propagate)

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
		removeTypeConfig(ctx, "v1", "Secret")
		Eventually(typeStatusMode(ctx, "v1", "Secret")).Should(Equal(testModeMisssing))

		// Give foo another secret.
		makeObject(ctx, "Secret", fooName, "foo-sec-2")
		// Foo should have "foo-sec-2" because we created it there.
		Eventually(hasObject(ctx, "Secret", fooName, "foo-sec-2")).Should(BeTrue())
		// "foo-sec-2" should not propagate from foo to bar because the reconciliation request is ignored.
		Consistently(hasObject(ctx, "Secret", barName, "foo-sec-2")()).Should(BeFalse(), "foo-sec-2 should not propagate to %s because propagation's been disabled", barName)

	})

	It("should reconcile after adding a new crd to the apiserver", func() {
		// Add a config for a type that hasn't been defined yet.
		addToHNCConfig(ctx, "stable.example.com/v1", "CronTab", api.Propagate)

		// The corresponding object reconciler should not be created because the type does not exist.
		Eventually(getHNCConfigCondition(ctx, api.ObjectReconcilerCreationFailed)).
			Should(ContainSubstring("stable.example.com/v1, Kind=CronTab"))

		// Add the CRD for CronTab to the apiserver.
		createCronTabCRD(ctx)

		// The object reconciler for CronTab should be created successfully, which means all conditions
		// should be cleared.
		Eventually(getHNCConfigCondition(ctx, api.ObjectReconcilerCreationFailed)).Should(Equal(""))

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

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(1))

		deleteObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(0))
	})

	It("should set NumPropagatedObjects back to 0 after switching from propagate to remove mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		setParent(ctx, barName, fooName)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(1))

		setTypeConfig(ctx, "v1", "LimitRange", api.Remove)

		Eventually(getNumPropagatedObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(0))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	It("should set NumSourceObjects for a type in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(1))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	// If a mode is unset, it is treated as `propagate` by default, in which case we will also compute NumSourceObjects
	It("should set NumSourceObjects for a type with unset mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", "")
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(1))

		// TODO: Delete objects created via makeObject after each test case.
		deleteObject(ctx, "LimitRange", fooName, "foo-lr")
	})

	It("should decrement NumSourceObjects correctly after deleting an object of a type in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Propagate)
		makeObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(1))

		deleteObject(ctx, "LimitRange", fooName, "foo-lr")

		Eventually(getNumSourceObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(Equal(0))
	})

	It("should not set NumSourceObjects for a type not in propagate mode", func() {
		addToHNCConfig(ctx, "v1", "LimitRange", api.Remove)

		Eventually(hasNumSourceObjects(ctx, "v1", "LimitRange"), countUpdateTime).Should(BeFalse())
	})
})

func typeSpecMode(ctx context.Context, apiVersion, kind string) func() api.SynchronizationMode {
	return func() api.SynchronizationMode {
		config, err := getHNCConfig(ctx)
		if err != nil {
			return (api.SynchronizationMode)(err.Error())
		}
		for _, t := range config.Spec.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				return t.Mode
			}
		}
		return testModeMisssing
	}
}

func typeStatusMode(ctx context.Context, apiVersion, kind string) func() api.SynchronizationMode {
	return func() api.SynchronizationMode {
		config, err := getHNCConfig(ctx)
		if err != nil {
			return (api.SynchronizationMode)(err.Error())
		}
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

func getHNCConfigCondition(ctx context.Context, code api.HNCConfigurationCode) func() string {
	return getNamedHNCConfigCondition(ctx, api.HNCConfigSingleton, code)
}

func getNamedHNCConfigCondition(ctx context.Context, nm string, code api.HNCConfigurationCode) func() string {
	return func() string {
		c, err := getHNCConfigWithName(ctx, nm)
		if err != nil {
			return err.Error()
		}
		msg := ""
		for _, cond := range c.Status.Conditions {
			if cond.Code == code {
				msg += cond.Msg + "\n"
			}
		}
		return msg
	}
}

// setTypeConfig is usually what you should call. It updates the config *and* waits for the
// config to take effect, as shown by the status.
//
// If you're making a change that won't be reflected in the status (e.g. removing Role), call
// updateTypeConfig, which doesn't confirm that the mode's taken effect.
func setTypeConfig(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	updateTypeConfigWithOffset(1, ctx, apiVersion, kind, mode)
	EventuallyWithOffset(1, typeStatusMode(ctx, apiVersion, kind)).Should(Equal(mode), "While setting type config for %s/%s to %s", apiVersion, kind, mode)
}

// updateTypeConfig is like setTypeConfig but it doesn't wait to confirm that the change was
// successful.
func updateTypeConfig(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	updateTypeConfigWithOffset(1, ctx, apiVersion, kind, mode)
}

func updateTypeConfigWithOffset(offset int, ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	EventuallyWithOffset(offset+1, func() error {
		// Get the existing spec from the apiserver
		c, err := getHNCConfig(ctx)
		if err != nil {
			return err
		}

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
		GinkgoT().Logf("Changing type config of %s/%s to %s", apiVersion, kind, mode)
		return updateHNCConfig(ctx, c)
	}).Should(Succeed(), "While updating type config for %s/%s to %s", apiVersion, kind, mode)
}

func unsetTypeConfig(ctx context.Context, apiVersion, kind string) {
	removeTypeConfigWithOffset(1, ctx, apiVersion, kind)
	Eventually(typeStatusMode(ctx, apiVersion, kind)).Should(Equal(testModeMisssing))
}

func removeTypeConfig(ctx context.Context, apiVersion, kind string) {
	removeTypeConfigWithOffset(1, ctx, apiVersion, kind)
}

func removeTypeConfigWithOffset(offset int, ctx context.Context, apiVersion, kind string) {
	EventuallyWithOffset(offset+1, func() error {
		c, err := getHNCConfig(ctx)
		if err != nil {
			return err
		}
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
		GinkgoT().Logf("Removing type config for %s/%s", apiVersion, kind)
		c.Spec.Types[i] = c.Spec.Types[len(c.Spec.Types)-1]
		c.Spec.Types = c.Spec.Types[:len(c.Spec.Types)-1]
		return updateHNCConfig(ctx, c)
	}).Should(Succeed(), "While removing type config for %s/%s", apiVersion, kind)
}

func removeTypeConfigWithMode(ctx context.Context, apiVersion, kind string, mode api.SynchronizationMode) {
	EventuallyWithOffset(1, func() error {
		c, err := getHNCConfig(ctx)
		if err != nil {
			return err
		}
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
	}).Should(Succeed(), "While removing type config for %s/%s with mode %s", apiVersion, kind, mode)
}

func createCronTabCRD(ctx context.Context) {
	crontab := apiextensions.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "crontabs.stable.example.com",
		},
		Spec: apiextensions.CustomResourceDefinitionSpec{
			Group: "stable.example.com",
			Versions: []apiextensions.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
			Names: apiextensions.CustomResourceDefinitionNames{
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
		c, err := getHNCConfig(ctx)
		if err != nil {
			return -1, err
		}
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
		c, err := getHNCConfig(ctx)
		if err != nil {
			return false, err
		}
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
		c, err := getHNCConfig(ctx)
		if err != nil {
			return -1, err
		}
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
