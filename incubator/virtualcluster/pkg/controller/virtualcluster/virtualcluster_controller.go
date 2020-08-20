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
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	strutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/strings"
	vcmanager "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/vcmanager"
)

var log = logf.Log.WithName("virtualcluster-controller")

// Add creates a new VirtualCluster Controller and adds it to the Manager with
// default RBAC. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr *vcmanager.VirtualClusterManager, masterProvisioner string) error {
	rcl, err := newReconciler(mgr, masterProvisioner)
	if err != nil {
		return err
	}
	return add(mgr, rcl)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, masterProv string) (reconcile.Reconciler, error) {
	var (
		mp  MasterProvisioner
		err error
	)
	switch masterProv {
	case "native":
		mp, err = NewMasterProvisionerNative(mgr)
		if err != nil {
			return nil, err
		}
	case "aliyun":
		mp, err = NewMasterProvisionerAliyun(mgr)
		if err != nil {
			return nil, err
		}
	}

	return &ReconcileVirtualCluster{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		mp:     mp,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr *vcmanager.VirtualClusterManager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualcluster-controller",
		mgr, controller.Options{
			MaxConcurrentReconciles: mgr.MaxConcurrentReconciles,
			Reconciler:              r})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualCluster
	err = c.Watch(&source.Kind{
		Type: &tenancyv1alpha1.VirtualCluster{}},
		&handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualCluster{}

// ReconcileVirtualCluster reconciles a VirtualCluster object
type ReconcileVirtualCluster struct {
	client.Client
	scheme *runtime.Scheme
	mp     MasterProvisioner
}

// Reconcile reads that state of the cluster for a VirtualCluster object and makes changes based on the state read
// and what is in the VirtualCluster.Spec
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
func (r *ReconcileVirtualCluster) Reconcile(request reconcile.Request) (rncilRslt reconcile.Result, err error) {
	log.Info("reconciling VirtualCluster...")
	vc := &tenancyv1alpha1.VirtualCluster{}
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
			if err = kubeutil.RetryUpdateVCStatusOnConflict(context.TODO(), r, vc, log); err != nil {
				return
			}
			log.Info("a finalizer has been registered for the VirtualCluster CRD", "finalizer", vcFinalizerName)
		}
	} else {
		// The VirtualCluster is being deleted
		if strutil.ContainString(vc.ObjectMeta.Finalizers, vcFinalizerName) {
			// delete the control plane
			log.Info("VirtualCluster is being deleted, finalizer will be activated", "vc-name", vc.Name, "finalizer", vcFinalizerName)
			// block if fail to delete VC
			if err = r.mp.DeleteVirtualCluster(vc); err != nil {
				log.Error(err, "fail to delete virtualcluster", "vc-name", vc.Name)
				return
			}
			// remove finalizer from the list and update it.
			vc.ObjectMeta.Finalizers = strutil.RemoveString(vc.ObjectMeta.Finalizers, vcFinalizerName)
			err = kubeutil.RetryUpdateVCStatusOnConflict(context.TODO(), r, vc, log)
		}
		return
	}

	// reconcile VirtualCluster (vc) based on vc status
	// NOTE: vc status is required by other components (e.g. syncer need to
	// know the vc status in order to setup connection to the tenant master)
	switch vc.Status.Phase {
	case "":
		// set vc status as ClusterPending if no status is set
		log.Info("will create a VirtualCluster", "vc", vc.Name)
		// will retry three times
		kubeutil.SetVCStatus(vc, tenancyv1alpha1.ClusterPending,
			"retry: 3", "ClusterCreating")
		err = kubeutil.RetryUpdateVCStatusOnConflict(context.TODO(), r, vc, log)
		return
	case tenancyv1alpha1.ClusterPending:
		// create new virtualcluster when vc is pending
		log.Info("VirtualCluster is pending", "vc", vc.Name)
		retryTimes, _ := strconv.Atoi(strings.TrimSpace(strings.Split(vc.Status.Message, ":")[1]))
		if retryTimes > 0 {
			err = r.mp.CreateVirtualCluster(vc)
			if err != nil {
				log.Error(err, "fail to create virtualcluster", "vc", vc.GetName(), "retrytimes", retryTimes)
				errReason := fmt.Sprintf("fail to create virtualcluster(%s): %s", vc.GetName(), err)
				errMsg := fmt.Sprintf("retry: %d", retryTimes-1)
				kubeutil.SetVCStatus(vc, tenancyv1alpha1.ClusterPending, errMsg, errReason)
			} else {
				kubeutil.SetVCStatus(vc, tenancyv1alpha1.ClusterRunning,
					"tenant master is running", "TenantMasterRunning")
			}
		} else {
			kubeutil.SetVCStatus(vc, tenancyv1alpha1.ClusterError,
				"fail to create virtualcluster", "TenantMasterError")
		}

		err = kubeutil.RetryUpdateVCStatusOnConflict(context.TODO(), r, vc, log)
		return
	case tenancyv1alpha1.ClusterRunning:
		log.Info("VirtualCluster is running", "vc", vc.GetName())
		return
	case tenancyv1alpha1.ClusterError:
		log.Info("fail to create virtualcluster", "vc", vc.GetName())
		return
	default:
		err = fmt.Errorf("unknown vc phase: %s", vc.Status.Phase)
		return
	}
}
