package foresttest

import (
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/forest"
)

// Create creates a forest with len(desc) namespaces, consecutively named a, b, etc. Each entry in
// the string defines both who the parent of the respective namespace is, as well as whether it's a
// subnamespace as well:
// * If the string element is a hyphen ('-'), the namespace is a root.
// * If the string element is a lowercase letter, the referenced namespace is a normal parent.
//   * If the referenced namespace is not in len(desc), it will not exist and we'll set
//   CritParentMissing on the child.
// * If the string element is an uppercase letter, that namespaces will be the parent and the
// *current* namespace will be a subnamespace.
//
// Examples:
// * "-"   -> creates a single namespace "a" that is a root
// * "-a"  -> creates the tree a <- b
// * "-A"  -> creates the tree a <- b where b is a subnamespace of a
// * "z"   -> creates the tree z <- a where z does not exist and a has CritParentMissing
// * "-aa" -> creates namespace `a` with two children, `b` and `c`
// * "-aA" -> as above, but c is a subnamespace and b is a full namespace
// * "ba"  -> creates a cycle
// * "-aa-dd" -> creates two trees, one with root `a` and children `b` and `c`, the other with root
//               `d` and children `e` and `f`
func Create(desc string) *forest.Forest {
	const upper = 'A'
	const lower = 'a'
	const toLower = (lower - upper)
	f := forest.NewForest()

	// First, create all legit namespaces
	for i, _ := range desc {
		nm := string(lower + byte(i))
		ns := f.Get(nm)
		ns.SetExists()
	}

	// Then, set all parents
	for i, pnm := range desc {
		if pnm == '-' {
			continue
		}
		ns := f.Get(string(lower + byte(i)))
		if pnm < lower {
			ns.IsSub = true
			pnm += toLower
		}
		pns := f.Get(string(pnm))
		ns.SetParent(pns)
		if !pns.Exists() {
			ns.SetLocalCondition(api.CritParentMissing, "no parent")
		}
		for _, cnm := range ns.CycleNames() {
			f.Get(cnm).SetLocalCondition(api.CritCycle, "in cycle")
		}
	}

	return f
}
