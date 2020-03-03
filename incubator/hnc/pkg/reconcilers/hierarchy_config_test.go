package reconcilers_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var _ = Describe("Hierarchy", func() {
	ctx := context.Background()

	var (
		fooName string
		barName string
	)

	BeforeEach(func() {
		fooName = createNS(ctx, "foo")
		barName = createNS(ctx, "bar")
	})

	It("should set a child on the parent", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(func() []string {
			barHier := getHierarchy(ctx, barName)
			return barHier.Status.Children
		}).Should(Equal([]string{fooName}))
	})

	It("should set CritParentMissing condition if the parent is missing", func() {
		// Set up the parent-child relationship
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = "brumpf"
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(true))
	})

	It("should unset CritParentMissing condition if the parent is later created", func() {
		// Set up the parent-child relationship with the missing name
		brumpfName := createNSName("brumpf")
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(true))

		// Create the missing parent
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())

		// Ensure the condition is resolved on the child
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(false))

		// Ensure the child is listed on the parent
		Eventually(func() []string {
			brumpfHier := getHierarchy(ctx, brumpfName)
			return brumpfHier.Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set CritAncestor condition if any ancestor has critical condition", func() {
		// Set up the parent-child relationship
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = "brumpf"
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(true))

		// Set bar as foo's parent
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritAncestor)).Should(Equal(true))
	})

	It("should unset CritAncestor condition if critical conditions in ancestors are gone", func() {
		// Set up the parent-child relationship with the missing name
		brumpfName := createNSName("brumpf")
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = brumpfName
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(true))

		// Set bar as foo's parent
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritAncestor)).Should(Equal(true))

		// Create the missing parent
		brumpfNS := &corev1.Namespace{}
		brumpfNS.Name = brumpfName
		Expect(k8sClient.Create(ctx, brumpfNS)).Should(Succeed())

		// Ensure the condition is resolved on the child
		Eventually(hasCondition(ctx, barName, api.CritParentMissing)).Should(Equal(false))

		// Ensure the child is listed on the parent
		Eventually(func() []string {
			brumpfHier := getHierarchy(ctx, brumpfName)
			return brumpfHier.Status.Children
		}).Should(Equal([]string{barName}))

		// Ensure foo is enqueued and thus get CritAncestor condition updated after
		// critical conditions are resolved in bar.
		Eventually(hasCondition(ctx, fooName, api.CritAncestor)).Should(Equal(false))
	})

	It("should set CritParentInvalid condition if a self-cycle is detected", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = fooName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritParentInvalid)).Should(Equal(true))
	})

	It("should set CritParentInvalid condition if a cycle is detected", func() {
		// Set up initial hierarchy
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))

		// Break it
		fooHier := getHierarchy(ctx, fooName)
		fooHier.Spec.Parent = barName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritParentInvalid)).Should(Equal(true))
	})

	It("should create a child namespace if requested", func() {
		if enableHNSReconciler {
			return
		}
		// Create a namespace with a required child
		fooHier := newHierarchy(fooName)
		fooHier.Spec.RequiredChildren = []string{barName}
		updateHierarchy(ctx, fooHier)

		// Verify that it exists
		Eventually(func() string {
			return getHierarchy(ctx, barName).Spec.Parent
		}).Should(Equal(fooName))
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))
	})

	It("should set RequiredChildConflict condition if a required child cannot be set", func() {
		if enableHNSReconciler {
			return
		}
		bazName := createNS(ctx, "baz")

		// Make baz a child of foo
		bazHier := newHierarchy(bazName)
		bazHier.Spec.Parent = fooName
		updateHierarchy(ctx, bazHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{bazName}))

		// Try to also make baz a required child of bar
		barHier := newHierarchy(barName)
		barHier.Spec.RequiredChildren = []string{bazName}
		updateHierarchy(ctx, barHier)

		// Verify that bar is reporting the conflict, but foo and bar are not.
		Eventually(hasCondition(ctx, fooName, "")).Should(Equal(false))
		Eventually(hasCondition(ctx, bazName, "")).Should(Equal(false))
		want := &api.Condition{
			Code:    api.RequiredChildConflict,
			Affects: []api.AffectedObject{{Version: "v1", Kind: "Namespace", Name: bazName}},
		}
		Eventually(getCondition(ctx, barName, api.RequiredChildConflict)).Should(Equal(want))
	})

	It("should clear RequiredChildConflict condition if the parent removes the required child", func() {
		if enableHNSReconciler {
			return
		}
		bazName := createNS(ctx, "baz")

		// Make baz a child of foo
		bazHier := newHierarchy(bazName)
		bazHier.Spec.Parent = fooName
		updateHierarchy(ctx, bazHier)
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{bazName}))

		// Try to also make baz a required child of bar
		barHier := newHierarchy(barName)
		barHier.Spec.RequiredChildren = []string{bazName}
		updateHierarchy(ctx, barHier)

		// Wait for bar to report the conflict.
		Eventually(hasCondition(ctx, barName, "")).Should(Equal(true))

		// Remove the required child from bar and verify that the condition clears.
		barHier = getHierarchy(ctx, barName) // because it's changed since the last time we updated it
		barHier.Spec.RequiredChildren = nil
		updateHierarchy(ctx, barHier)
		Eventually(hasCondition(ctx, barName, "")).Should(Equal(false))
	})

	It("should have a tree label", func() {
		// Make bar a child of foo
		barHier := newHierarchy(barName)
		barHier.Spec.Parent = fooName
		updateHierarchy(ctx, barHier)
		// First, verify bar is a child of foo
		Eventually(func() []string {
			return getHierarchy(ctx, fooName).Status.Children
		}).Should(Equal([]string{barName}))
		// Verify that bar has a tree label related to foo
		Eventually(func() bool {
			barNS := getNamespace(ctx, barName)
			_, ok := barNS.GetLabels()[fooName+".tree."+api.MetaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			barNS := getNamespace(ctx, barName)
			val, _ := barNS.GetLabels()[fooName+".tree."+api.MetaGroup+"/depth"]
			return val
		}).Should(Equal("1"))
		// Verify that bar has a tree label related to bar itself
		Eventually(func() bool {
			barNS := getNamespace(ctx, barName)
			_, ok := barNS.GetLabels()[barName+".tree."+api.MetaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			barNS := getNamespace(ctx, barName)
			val, _ := barNS.GetLabels()[barName+".tree."+api.MetaGroup+"/depth"]
			return val
		}).Should(Equal("0"))
		// Verify that foo has a tree label related to foo itself
		Eventually(func() bool {
			fmt.Println(getHierarchy(ctx, fooName))
			fooNS := getNamespace(ctx, fooName)
			_, ok := fooNS.GetLabels()[fooName+".tree."+api.MetaGroup+"/depth"]
			return ok
		}).Should(BeTrue())
		// Verify the label value
		Eventually(func() string {
			fooNS := getNamespace(ctx, fooName)
			val, _ := fooNS.GetLabels()[fooName+".tree."+api.MetaGroup+"/depth"]
			return val
		}).Should(Equal("0"))
	})

	It("should update labels when parent is changed", func() {
		// Set up key-value pair for non-HNC label
		const keyName = "key"
		const valueName = "value"

		// Set up initial hierarchy
		bazName := createNSWithLabel(ctx, "baz", map[string]string{keyName: valueName})
		bazHier := newHierarchy(bazName)
		depthSuffix := fmt.Sprintf(".tree.%s/depth", api.MetaGroup)
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))

		// Make baz as a child of bar
		bazHier.Spec.Parent = barName
		updateHierarchy(ctx, bazHier)

		// Verify all labels on baz after set bar as parent
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, barName+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))

		// Change parent to foo
		bazHier.Spec.Parent = fooName
		updateHierarchy(ctx, bazHier)

		// Verify all labels on baz after change parent to foo
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, fooName+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, bazName, barName+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))
	})

	It("should update labels when parent is removed", func() {
		// Set up key-value pair for non-HNC label
		const keyName = "key"
		const valueName = "value"

		// Set up initial hierarchy
		bazName := createNSWithLabel(ctx, "baz", map[string]string{keyName: valueName})
		bazHier := newHierarchy(bazName)
		depthSuffix := fmt.Sprintf(".tree.%s/depth", api.MetaGroup)
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))

		// Make baz as a child of bar
		bazHier.Spec.Parent = barName
		updateHierarchy(ctx, bazHier)

		// Verify all labels on baz after set bar as parent
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, barName+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))

		// Remove parent from baz
		bazHier.Spec.Parent = ""
		updateHierarchy(ctx, bazHier)

		// Verify all labels on baz after parent removed
		Eventually(getLabel(ctx, bazName, bazName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, bazName, barName+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, bazName, keyName)).Should(Equal(valueName))
	})
})

func hasCondition(ctx context.Context, nm string, code api.Code) func() bool {
	return func() bool {
		conds := getHierarchy(ctx, nm).Status.Conditions
		if code == "" {
			return len(conds) > 0
		}
		for _, cond := range conds {
			if cond.Code == code {
				return true
			}
		}
		return false
	}
}

func getCondition(ctx context.Context, nm string, code api.Code) func() *api.Condition {
	return func() *api.Condition {
		conds := getHierarchy(ctx, nm).Status.Conditions
		for _, cond := range conds {
			if cond.Code == code {
				ret := cond.DeepCopy()
				ret.Msg = "" // don't want changes here to break tests
				return ret
			}
		}
		return nil
	}
}

func newHierarchy(nm string) *api.HierarchyConfiguration {
	hier := &api.HierarchyConfiguration{}
	hier.ObjectMeta.Namespace = nm
	hier.ObjectMeta.Name = api.Singleton
	return hier
}

func getHierarchy(ctx context.Context, nm string) *api.HierarchyConfiguration {
	return getHierarchyWithOffset(1, ctx, nm)
}

func getHierarchyWithOffset(offset int, ctx context.Context, nm string) *api.HierarchyConfiguration {
	snm := types.NamespacedName{Namespace: nm, Name: api.Singleton}
	hier := &api.HierarchyConfiguration{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, snm, hier)
	}).Should(Succeed())
	return hier
}

func getNamespace(ctx context.Context, nm string) *corev1.Namespace {
	return getNamespaceWithOffset(1, ctx, nm)
}

func getNamespaceWithOffset(offset int, ctx context.Context, nm string) *corev1.Namespace {
	nnm := types.NamespacedName{Name: nm}
	ns := &corev1.Namespace{}
	EventuallyWithOffset(offset+1, func() error {
		return k8sClient.Get(ctx, nnm, ns)
	}).Should(Succeed())
	return ns
}

func updateHierarchy(ctx context.Context, h *api.HierarchyConfiguration) {
	if h.CreationTimestamp.IsZero() {
		ExpectWithOffset(1, k8sClient.Create(ctx, h)).Should(Succeed())
	} else {
		ExpectWithOffset(1, k8sClient.Update(ctx, h)).Should(Succeed())
	}
}

func getLabel(ctx context.Context, from, label string) func() string {
	return func() string {
		ns := getNamespace(ctx, from)
		val, _ := ns.GetLabels()[label]
		return val
	}
}
