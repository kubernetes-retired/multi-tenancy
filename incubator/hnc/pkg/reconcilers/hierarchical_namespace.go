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
	"context"

	"github.com/go-logr/logr"
	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HierarchicalNamespaceReconciler reconciles HierarchicalNamespace CRs to make sure
// all the hierarchical namespaces are properly maintained.
type HierarchicalNamespaceReconciler struct {
	client.Client
	Log logr.Logger
}

// Reconcile sets up some basic variables and then calls the business logic.
// It currently handles basic creation of namespace and hierarchyconfig.
func (r *HierarchicalNamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// TODO report error state if the webhook is bypassed - see issue
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/459
	if ex[req.Namespace] {
		return ctrl.Result{}, nil
	}

	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)
	log.Info("Reconciling HNS")

	// Names of the self-serve subnamespace and the parent namespace.
	nm := req.Name
	pnm := req.Namespace

	// We want to trigger the hierarchyConfig reconciliation on the self-serve subnamespace, since
	// the parent's 'requiredChildren' field will be deprecated. The HierarchyConfig Reconciler
	// will do the real job of updating the hierarchy in the forest and creating namespaces and
	// hierarchyconfig (hc) instances.
	//
	// Please note that the self-serve subnamespace and its hc object don't exist on apiserver for now.
	// Just enqueuing an in-memory hc instance for hc reconciliation will lose the hc's parent field
	// since the hc reconciler will get nothing from the apiserver and create an empty in-memory hc.
	// Therefore, we need to create the hc instance on apiserver and thus its namespace. The creation of
	// the hc instance on apiserver will trigger the hc reconciler, who will update the forest and create
	// the parent hc instance if it doesn't already exist.
	// TODO: Add hnc.x-k8s.io/owner annotation to hierarchical namespaces. See issue -
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/473
	if err := r.writeNamespace(ctx, log, nm); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.writeHierarchy(ctx, log, nm, pnm); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *HierarchicalNamespaceReconciler) writeNamespace(ctx context.Context, log logr.Logger, nm string) error {
	inst := &corev1.Namespace{}
	inst.ObjectMeta.Name = nm

	log.Info("Creating namespace on apiserver")
	if err := r.Create(ctx, inst); err != nil {
		log.Error(err, "while creating on apiserver")
		return err
	}
	return nil
}

func (r *HierarchicalNamespaceReconciler) writeHierarchy(ctx context.Context, log logr.Logger, nm, pnm string) error {
	inst := &api.HierarchyConfiguration{
		Spec: api.HierarchyConfigurationSpec{Parent: pnm},
	}
	inst.ObjectMeta.Name = api.Singleton
	inst.ObjectMeta.Namespace = nm

	log.Info("Creating singleton on apiserver")
	if err := r.Create(ctx, inst); err != nil {
		log.Error(err, "while creating on apiserver")
		return err
	}

	return nil
}

func (r *HierarchicalNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HierarchicalNamespace{}).
		Complete(r)
}
