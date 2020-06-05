package forest

import (
	"errors"
	"sort"
	"strings"

	"github.com/go-logr/logr"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// HasLocalCritCondition returns if the namespace itself has any local critical conditions, ignoring
// its ancestors. Any code with the "Crit" prefix is a critical condition.
func (ns *Namespace) HasLocalCritCondition() bool {
	for code, _ := range ns.conditions[api.AffectedObject{}] {
		codeNm := (string)(code)
		if strings.HasPrefix(codeNm, "Crit") {
			return true
		}
	}
	return false
}

// GetCritAncestor returns the name of the first ancestor with a critical condition, or the empty
// string if there are no such ancestors. It *can* return the name of the current namespace.
func (ns *Namespace) GetCritAncestor() string {
	if ns.HasLocalCritCondition() {
		return ns.name
	}
	if ns.Parent() == nil {
		return ""
	}
	return ns.Parent().GetCritAncestor()
}

// HasCondition returns true if there's a condition with the given object and code. If code is the
// empty string, it returns true if there's _any_ condition for the given object.
func (ns *Namespace) HasCondition(obj api.AffectedObject, code api.Code) bool {
	if _, exists := ns.conditions[obj]; !exists {
		// Nothing for this obj
		return false
	}
	if code == "" {
		// Something exists for this obj; we don't care what
		return true
	}
	_, exists := ns.conditions[obj][code]
	return exists
}

// ClearCondition clears conditions in the namespace for a single object. If `code` is non-empty, it
// only clears conditions with that code, otherwise it clears all conditions for that object. It
// should only be called by the code that also *sets* the conditions.
//
// It returns true if it made any changes, false otherwise.
func (ns *Namespace) ClearCondition(obj api.AffectedObject, code api.Code) bool {
	if !ns.HasCondition(obj, code) {
		return false
	}

	if code == "" {
		delete(ns.conditions, obj)
	} else {
		delete(ns.conditions[obj], code)
	}

	return true
}

// ClearLocalConditions clears the condition(s) on this namespace.
func (ns *Namespace) ClearLocalConditions() bool {
	return ns.ClearCondition(api.AffectedObject{}, "")
}

func (ns *Namespace) ClearObsoleteConditions(log logr.Logger) {
	// Load ancestors to check CCCAncestors
	isAnc := map[string]bool{}
	for _, anc := range ns.AncestryNames() {
		// The definition of CCCAncestor doesn't include the namespace itself
		if anc != ns.name {
			isAnc[anc] = true
		}
	}

	// Load the subtree to check CCCSubtree, including the namespace itself.
	isSubtree := map[string]bool{ns.name: true}
	for _, dsc := range ns.DescendantNames() {
		isSubtree[dsc] = true
	}

	// For each affected object, remove its condition if that object is no longer relevant.
	for obj, codes := range ns.conditions {
		for code, _ := range codes {
			switch api.ClearConditionCriteria[code] {
			case api.CCCManual:
				// nop - cleared manually
			case api.CCCAncestor:
				if !isAnc[obj.Namespace] {
					log.Info("Cleared obsolete condition from old ancestor", "obj", obj, "code", code)
					ns.ClearCondition(obj, code)
				}
			case api.CCCSubtree:
				if !isSubtree[obj.Namespace] {
					log.Info("Cleared obsolete condition from old descendant", "obj", obj, "code", code)
					ns.ClearCondition(obj, code)
				}
			default:
				err := errors.New("no ClearConditionCriterion")
				log.Error(err, "In clearObsoleteConditions", "code", code, "obj", obj)
			}
		}
	}
}

// SetCondition sets a condition for the specified object and code, returning true if it does not
// exist previously or if the message has changed.
//
// Returns true if the condition wasn't previously set
func (ns *Namespace) SetCondition(obj api.AffectedObject, code api.Code, msg string) bool {
	changed := false
	if _, existed := ns.conditions[obj]; !existed {
		changed = true
		ns.conditions[obj] = map[api.Code]string{}
	}

	if oldMsg, existed := ns.conditions[obj][code]; !existed || msg != oldMsg {
		changed = true
		ns.conditions[obj][code] = msg
	}

	return changed
}

// SetLocalCondition sets a condition that applies to the current namespace.
func (ns *Namespace) SetLocalCondition(code api.Code, msg string) bool {
	return ns.SetCondition(api.AffectedObject{}, code, msg)
}

// Conditions returns a list of conditions in the namespace in the format expected by the API.
func (ns *Namespace) Conditions() []api.Condition {
	// Treat the code/msg combination as a combined key.
	type codeMsg struct {
		code api.Code
		msg  string
	}

	// Reorder so that the objects are grouped by code and message
	byCM := map[codeMsg][]api.AffectedObject{}
	for obj, codes := range ns.conditions {
		for code, msg := range codes {
			cm := codeMsg{code: code, msg: msg}
			byCM[cm] = append(byCM[cm], obj)
		}
	}

	// Flatten into a list of conditions
	conds := []api.Condition{}
	for cm, objs := range byCM {
		// If the only affected object is unnamed (e.g., it refers to the current namespace), omit it.
		c := api.Condition{Code: cm.code, Msg: cm.msg}
		if len(objs) > 0 && objs[0].Name != "" {
			api.SortAffectedObjects(objs)
			c.Affects = objs
		}
		conds = append(conds, c)
	}

	sort.Slice(conds, func(i, j int) bool {
		if conds[i].Code != conds[j].Code {
			return conds[i].Code < conds[j].Code
		}
		return conds[i].Msg < conds[j].Msg
	})

	if len(conds) == 0 {
		conds = nil // prevent anything from appearing in the status
	}
	return conds
}
