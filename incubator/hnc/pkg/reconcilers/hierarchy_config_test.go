package reconcilers_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

const (
	depthSuffix = ".tree." + api.MetaGroup + "/depth"
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
		Eventually(hasChild(ctx, barName, fooName)).Should(Equal(true))
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
		Eventually(hasChild(ctx, brumpfName, barName)).Should(Equal(true))
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
		Eventually(hasChild(ctx, brumpfName, barName)).Should(Equal(true))

		// Ensure foo is enqueued and thus get CritAncestor condition updated after
		// critical conditions are resolved in bar.
		Eventually(hasCondition(ctx, fooName, api.CritAncestor)).Should(Equal(false))
	})

	It("should set CritCycle condition if a self-cycle is detected", func() {
		fooHier := newHierarchy(fooName)
		fooHier.Spec.Parent = fooName
		updateHierarchy(ctx, fooHier)
		Eventually(hasCondition(ctx, fooName, api.CritCycle)).Should(Equal(true))
	})

	It("should set CritCycle condition if a cycle is detected", func() {
		// Set up initial hierarchy
		setParent(ctx, barName, fooName)
		Eventually(hasChild(ctx, fooName, barName)).Should(Equal(true))

		// Break it
		setParent(ctx, fooName, barName)
		Eventually(hasCondition(ctx, fooName, api.CritCycle)).Should(Equal(true))
		Eventually(hasCondition(ctx, barName, api.CritCycle)).Should(Equal(true))

		// Fix it
		setParent(ctx, fooName, "")
		Eventually(hasCondition(ctx, fooName, api.CritCycle)).Should(Equal(false))
		Eventually(hasCondition(ctx, barName, api.CritCycle)).Should(Equal(false))
	})

	It("should have a tree label", func() {
		// Make bar a child of foo
		setParent(ctx, barName, fooName)

		// Verify all the labels
		Eventually(getLabel(ctx, barName, fooName+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, barName, barName+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, fooName, fooName+depthSuffix)).Should(Equal("0"))
	})

	It("should update labels when parent is changed", func() {
		// Set up key-value pair for non-HNC label
		const keyName = "key"
		const valueName = "value"

		// Set up initial hierarchy
		bazName := createNSWithLabel(ctx, "baz", map[string]string{keyName: valueName})
		bazHier := newHierarchy(bazName)
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

	It("should clear tree labels that are involved in a cycle, except the first one", func() {
		// Create the initial tree:
		// a(0) -+- b(1) -+- d(3) --- f(5)
		//       +- c(2)  +- e(4)
		nms := createNSes(ctx, 6)
		setParent(ctx, nms[1], nms[0])
		setParent(ctx, nms[2], nms[0])
		setParent(ctx, nms[3], nms[1])
		setParent(ctx, nms[4], nms[1])
		setParent(ctx, nms[5], nms[3])

		// Check all labels
		Eventually(getLabel(ctx, nms[0], nms[0]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[1]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[0]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[2], nms[2]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[2], nms[0]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[3], nms[3]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[3], nms[1]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[3], nms[0]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[4], nms[4]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[4], nms[1]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[4], nms[0]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[5], nms[5]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[5], nms[3]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[5], nms[1]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[5], nms[0]+depthSuffix)).Should(Equal("3"))

		// Create a cycle from a(0) to d(3) and check all labels.
		setParent(ctx, nms[0], nms[3])
		Eventually(getLabel(ctx, nms[0], nms[0]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[1]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[0]+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, nms[2], nms[2]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[2], nms[0]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[3], nms[3]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[3], nms[1]+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, nms[3], nms[0]+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, nms[4], nms[4]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[4], nms[1]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[4], nms[0]+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, nms[5], nms[5]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[5], nms[3]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[5], nms[1]+depthSuffix)).Should(Equal(""))
		Eventually(getLabel(ctx, nms[5], nms[0]+depthSuffix)).Should(Equal(""))

		// Fix the cycle and ensure that everything's restored
		setParent(ctx, nms[0], "")
		Eventually(getLabel(ctx, nms[0], nms[0]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[1]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[1], nms[0]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[2], nms[2]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[2], nms[0]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[3], nms[3]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[3], nms[1]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[3], nms[0]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[4], nms[4]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[4], nms[1]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[4], nms[0]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[5], nms[5]+depthSuffix)).Should(Equal("0"))
		Eventually(getLabel(ctx, nms[5], nms[3]+depthSuffix)).Should(Equal("1"))
		Eventually(getLabel(ctx, nms[5], nms[1]+depthSuffix)).Should(Equal("2"))
		Eventually(getLabel(ctx, nms[5], nms[0]+depthSuffix)).Should(Equal("3"))
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

func updateHierarchy(ctx context.Context, h *api.HierarchyConfiguration) {
	if h.CreationTimestamp.IsZero() {
		ExpectWithOffset(1, k8sClient.Create(ctx, h)).Should(Succeed())
	} else {
		ExpectWithOffset(1, k8sClient.Update(ctx, h)).Should(Succeed())
	}
}

func tryUpdateHierarchy(ctx context.Context, h *api.HierarchyConfiguration) error {
	if h.CreationTimestamp.IsZero() {
		return k8sClient.Create(ctx, h)
	} else {
		return k8sClient.Update(ctx, h)
	}
}

func getLabel(ctx context.Context, from, label string) func() string {
	return func() string {
		ns := getNamespace(ctx, from)
		val, _ := ns.GetLabels()[label]
		return val
	}
}

func hasChild(ctx context.Context, nm, cnm string) func() bool {
	return func() bool {
		children := getHierarchy(ctx, nm).Status.Children
		for _, c := range children {
			if c == cnm {
				return true
			}
		}
		return false
	}
}

// Namespaces are named "a-<rand>", "b-<rand>", etc
func createNSes(ctx context.Context, num int) []string {
	nms := []string{}
	for i := 0; i < num; i++ {
		nm := createNS(ctx, string('a'+i))
		nms = append(nms, nm)
	}
	return nms
}
