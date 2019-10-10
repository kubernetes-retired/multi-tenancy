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

package clusterversion

import (
	"context"
	"fmt"
	"reflect"

	tenancyv1alpha1 "github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ClusterVersion Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterVersion{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterversion-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterVersion
	err = c.Watch(&source.Kind{Type: &tenancyv1alpha1.ClusterVersion{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterVersion{}

// ReconcileClusterVersion reconciles a ClusterVersion object
type ReconcileClusterVersion struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterVersion object and makes changes based on the state read
// and what is in the ClusterVersion.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions/status,verbs=get;update;patch
func (r *ReconcileClusterVersion) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterVersion instance
	log.Info("reconciling cluster version")
	cv := &tenancyv1alpha1.ClusterVersion{}
	err := r.Get(context.TODO(), request.NamespacedName, cv)
	if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, ignoreNotFound(err)
	}
	log.Info(fmt.Sprintf("%v", reflect.TypeOf(cv.Spec.ControllerManager.StatefulSet)))

	// Register finalizers
	cvf := "clusterVersion.v1.finalizers"

	if cv.ObjectMeta.DeletionTimestamp.IsZero() {
		// the object has not been deleted yet, registers the finalizers
		if containString(cv.ObjectMeta.Finalizers, cvf) == false {
			cv.ObjectMeta.Finalizers = append(cv.ObjectMeta.Finalizers, cvf)
			log.Info("register finalizer for ClusterVersion", "finalizer", cvf)
			if err := r.Update(context.Background(), cv); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// the object is being deleted, star the finalizer
		if containString(cv.ObjectMeta.Finalizers, cvf) == true {
			// the finalizer logic
			log.Info("a ClusterVersion object is deleted", "ClusterVersion", cv.Name)

			// remove the finalizer after done
			cv.ObjectMeta.Finalizers = removeString(cv.ObjectMeta.Finalizers, cvf)
			if err := r.Update(context.Background(), cv); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

func ignoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func containString(sli []string, s string) bool {
	for _, str := range sli {
		if str == s {
			return true
		}
	}
	return false
}

func removeString(sli []string, s string) (newSli []string) {
	for _, str := range sli {
		if str == s {
			continue
		}
		newSli = append(newSli, str)
	}
	return
}
