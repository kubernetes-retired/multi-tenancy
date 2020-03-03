/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcilers

import (
	"github.com/go-logr/logr"
	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HierarchicalNamespaceReconciler reconciles HierarchicalNamespace CRs to make sure
// all the hierarchical namespaces are properly maintained.
type HierarchicalNamespaceReconciler struct {
	client.Client
	Log logr.Logger

	forest *forest.Forest
	hcr    *HierarchyConfigReconciler
}

// Reconcile sets up some basic variables and then calls the business logic. It currently
// only handles the creation of the namespaces but no deletion or state reporting yet.
func (r *HierarchicalNamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// TODO report error state if the webhook is bypassed - see issue
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/459
	if ex[req.Namespace] {
		return ctrl.Result{}, nil
	}

	log := r.Log.WithValues("trigger", req.NamespacedName)
	log.Info("Reconciling HNS")

	// Names of the hierarchical namespace and the current namespace.
	nm := req.Name
	pnm := req.Namespace

	// Set RequiredChildOf (Owner) in the forest of the hierarchical namespace to the
	// current namespace.
	// TODO rename RequiredChildOf to Owner in the forest. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/469
	r.forest.Get(nm).RequiredChildOf = pnm

	// Enqueue the in-momery hierarchyConfig instance of the hierarchical namespace.
	// The hierarchyConfig reconciler will create the namespace and hierarchyConfig
	// instances on apiserver accordingly.
	reason := "new/updated hierarchical namespace"
	r.hcr.enqueueAffected(log, reason, nm)

	// TODO sync with forest to report conflicts in hns.Status.State. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/487

	return ctrl.Result{}, nil
}

func (r *HierarchicalNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HierarchicalNamespace{}).
		Complete(r)
}
