package forest

import (
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

// HasLocalCritCondition returns if the namespace itself has any local critical conditions, ignoring
// its ancestors. Any code with the "Crit" prefix is a critical condition.
func (ns *Namespace) HasLocalCritCondition() bool {
	for _, cond := range ns.conditions {
		if cond.Type == api.ConditionActivitiesHalted && cond.Reason != api.ReasonAncestor {
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

// SetCondition adds a new condition to the current condition list.
func (ns *Namespace) SetCondition(tp, reason, msg string) {
	oldCond := ns.conditions
	ns.conditions = append(oldCond, api.NewCondition(tp, reason, msg))
}

// ClearConditions set conditions to nil.
func (ns *Namespace) ClearConditions() {
	ns.conditions = nil
}

// Conditions returns a full list of the conditions in the namespace.
func (ns *Namespace) Conditions() []api.Condition {
	return ns.conditions
}
