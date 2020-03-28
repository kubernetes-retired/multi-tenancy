/*
Copyright 2019 The Kubernetes Authors.

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

package virtualcluster

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	strutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/util/strings"
	vcmanager "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/vcmanager"
)

var log = logf.Log.WithName("virtualcluster-controller")

// Add creates a new Virtualcluster Controller and adds it to the Manager with
// default RBAC. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr *vcmanager.VirtualclusterManager, masterProvisioner string) error {
	return add(mgr, newReconciler(mgr, masterProvisioner))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, masterProv string) reconcile.Reconciler {
	var mp MasterProvisioner
	switch masterProv {
	case "native":
		mp = NewMasterProvisionerNative(mgr)
	case "aliyun":
		mp = NewMasterProvisionerAliyun(mgr)
	}

	return &ReconcileVirtualcluster{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		mp:     mp,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr *vcmanager.VirtualclusterManager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualcluster-controller",
		mgr, controller.Options{
			MaxConcurrentReconciles: mgr.MaxConcurrentReconciles,
			Reconciler:              r})
	if err != nil {
		return err
	}

	// Watch for changes to Virtualcluster
	err = c.Watch(&source.Kind{
		Type: &tenancyv1alpha1.Virtualcluster{}},
		&handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualcluster{}

// ReconcileVirtualcluster reconciles a Virtualcluster object
type ReconcileVirtualcluster struct {
	client.Client
	scheme *runtime.Scheme
	mp     MasterProvisioner
}

// Reconcile reads that state of the cluster for a Virtualcluster object and makes changes based on the state read
// and what is in the Virtualcluster.Spec
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions/status,verbs=get
func (r *ReconcileVirtualcluster) Reconcile(request reconcile.Request) (rncilRslt reconcile.Result, err error) {
	log.Info("reconciling Virtualcluster...")
	vc := &tenancyv1alpha1.Virtualcluster{}
	err = r.Get(context.TODO(), request.NamespacedName, vc)
	if err != nil {
		// set NotFound error as nil
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return
	}

	vcFinalizerName := fmt.Sprintf("virtualcluster.finalizer.%s", r.mp.GetMasterProvisioner())

	if vc.ObjectMeta.DeletionTimestamp.IsZero() {
		if !strutil.ContainString(vc.ObjectMeta.Finalizers, vcFinalizerName) {
			vc.ObjectMeta.Finalizers = append(vc.ObjectMeta.Finalizers, vcFinalizerName)
			if err = r.Update(context.Background(), vc); err != nil {
				return
			}
			log.Info("finalizer registered", "finalizer", vcFinalizerName)
		}
	} else {
		// The Virtualcluster is being deleted
		if strutil.ContainString(vc.ObjectMeta.Finalizers, vcFinalizerName) {
			// delete the control plane
			log.Info("Virtualcluster is being deleted, finalizer will be activated", "vc-name", vc.Name, "finalizer", vcFinalizerName)
			// NOTE we don't want to block the reconciling process due to deletion error
			r.mp.DeleteVirtualCluster(vc)
			// remove finalizer from the list and update it.
			vc.ObjectMeta.Finalizers = strutil.RemoveString(vc.ObjectMeta.Finalizers, vcFinalizerName)
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updateErr := r.Update(context.TODO(), vc)
				if err = r.Get(context.TODO(), request.NamespacedName, vc); err != nil {
					log.Info("fail to get vc on update failure", "error", err.Error())
				}
				return updateErr
			})
		}
		return
	}

	// reconcile Virtualcluster (vc) based on vc status
	// NOTE: vc status is required by other components (e.g. syncer need to
	// know the vc status in order to setup connection to tenant master)
	switch vc.Status.Phase {
	case "":
		// set vc status as ClusterPending if no status is set
		log.Info("will create a Virtualcluster", "vc", vc.Name)
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			vc.Status.Phase = tenancyv1alpha1.ClusterPending
			vc.Status.Message = "creating virtual cluster..."
			vc.Status.Reason = "ClusterCreating"
			updateErr := r.Update(context.TODO(), vc)
			if err = r.Get(context.TODO(), request.NamespacedName, vc); err != nil {
				log.Info("fail to get vc on update failure", "error", err.Error())
			}
			return updateErr
		})
		return
	case tenancyv1alpha1.ClusterPending:
		// create new virtualcluster when vc is pending
		log.Info("Virtualcluster is pending", "vc", vc.Name)

		err = r.mp.CreateVirtualCluster(vc)
		if err != nil {
			return
		}
		// all components are ready, update vc status
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			vc.Status.Phase = "Running"
			vc.Status.Message = "tenant master is running"
			vc.Status.Reason = "TenantMasterRunning"
			vc.Status.Conditions = append(vc.Status.Conditions, tenancyv1alpha1.ClusterCondition{
				LastTransitionTime: metav1.NewTime(time.Now()),
				Message:            fmt.Sprintf("virtualcluster(%s) starts running", vc.GetName()),
			})
			updateErr := r.Update(context.TODO(), vc)
			if err = r.Get(context.TODO(), request.NamespacedName, vc); err != nil {
				log.Info("fail to get vc on update failure", "error", err.Error())
			}
			return updateErr
		})
		return
	case tenancyv1alpha1.ClusterRunning:
		log.Info("Virtualcluster is running", "vc", vc.Name)
		return
	default:
		err = fmt.Errorf("unknown vc phase: %s", vc.Status.Phase)
		return
	}
}
