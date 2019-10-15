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
	"errors"
	"fmt"

	tenancyv1alpha1 "github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/controller/kubeconfig"
	vcpki "github.com/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("virtualcluster-controller")

type CRKind int

const (
	virtualCluster CRKind = iota
	clusterVersion
	unknownCR
)

// Add creates a new Virtualcluster Controller and adds it to the Manager with
// default RBAC. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVirtualcluster{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Virtualcluster
	err = c.Watch(&source.Kind{Type: &tenancyv1alpha1.Virtualcluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterVersion
	err = c.Watch(&source.Kind{Type: &tenancyv1alpha1.ClusterVersion{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch Statefulsets created by Virtualcluster
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &tenancyv1alpha1.Virtualcluster{},
	})
	if err != nil {
		return err
	}

	// watch Services created by Virtualcluster
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &tenancyv1alpha1.Virtualcluster{},
	})

	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualcluster{}

// ReconcileVirtualcluster reconciles a Virtualcluster object
type ReconcileVirtualcluster struct {
	client.Client
	scheme          *runtime.Scheme
	clusterVersions map[string]*tenancyv1alpha1.ClusterVersion
	virtualClusters map[string]*tenancyv1alpha1.Virtualcluster
}

func (r *ReconcileVirtualcluster) checkCRKind(request *reconcile.Request) (runtime.Object, CRKind, error) {
	vc := &tenancyv1alpha1.Virtualcluster{}
	var err error
	err = r.Get(context.TODO(), request.NamespacedName, vc)
	if err == nil {
		log.Info("get vc", "vc-name", vc.Name)
		return vc, virtualCluster, nil
	}

	cv := &tenancyv1alpha1.ClusterVersion{}
	err = r.Get(context.TODO(), request.NamespacedName, cv)
	if err == nil {
		return cv, clusterVersion, nil
	}

	return nil, unknownCR, ignoreNotFound(err)
}

// createNamespace creates a namespace for the virtual cluster. Each
// virtual cluster will have a corresponding namespace whose name
// is same as the virtual cluster's name
func (r *ReconcileVirtualcluster) createNamespace(ns string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	return r.Create(context.TODO(), namespace)
}

// createPKI constructs the PKI (all crt/key pair and kubeconfig) for the virtual clusters. All csr and key will
// be stored as secrets in meta cluster
func (r *ReconcileVirtualcluster) createPKI(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (*vcpki.ClusterCAGroup, error) {
	caGroup := &vcpki.ClusterCAGroup{}
	// 1. create root ca, all components will share a single root ca
	rootCAPair, rootCAErr := vcpki.NewCertificateAuthority(&cert.Config{
		CommonName:   "kubernetes",
		Organization: []string{"kubernetes-sig.multi-tenancy.virtualcluster"},
	})
	if rootCAErr != nil {
		return nil, rootCAErr
	}
	caGroup.RootCA = rootCAPair

	etcdDomain := cv.GetEtcdDomain()

	// 2. create crt, key for etcd
	etcdCAPair, etcdCrtErr := vcpki.NewEtcdServerCrtAndKey(rootCAPair, etcdDomain)
	if etcdCrtErr != nil {
		return nil, etcdCrtErr
	}
	caGroup.ETCD = etcdCAPair

	// 3. create crt, key for apiserver
	apiserverDomain := cv.GetAPIServerDomain()
	apiserverCAPair, apiserverCrtErr :=
		vcpki.NewAPIServerCrtAndKey(rootCAPair, vc, apiserverDomain)
	if apiserverCrtErr != nil {
		return nil, apiserverCrtErr
	}
	caGroup.APIServer = apiserverCAPair

	// 4. create kubeconfig for controller-manager
	ctrlmgrKbCfg, cmKbCfgErr := kubeconfig.GenerateKubeconfig(
		"system:kube-controller-manager", vc.Name, rootCAPair, apiserverDomain)
	if cmKbCfgErr != nil {
		return nil, cmKbCfgErr
	}
	caGroup.CtrlMgrKbCfg = ctrlmgrKbCfg

	// 5. create kubeconfig for admin user
	adminKbCfg, adminKbCfgErr := kubeconfig.GenerateKubeconfig(
		"admin", vc.Name, rootCAPair, apiserverDomain)
	if adminKbCfgErr != nil {
		return nil, adminKbCfgErr
	}
	caGroup.AdminKbCfg = adminKbCfg

	// 6. create rsa key for service-account
	svcAcctCAPair, saCrtErr := vcpki.NewServiceAccountSigningKey()
	if saCrtErr != nil {
		return nil, saCrtErr
	}
	caGroup.ServiceAccountPrivateKey = svcAcctCAPair

	return caGroup, nil
}

func (r *ReconcileVirtualcluster) updateVirtualcluster(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (reconcile.Result, error) {
	log.Info("NOT IMPLEMENT YET")
	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualcluster) createVirtualcluster(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (reconcile.Result, error) {
	// 1. create ns
	if err := r.createNamespace(vc.Name); err != nil {
		return reconcile.Result{}, err
	}
	// 2. create pki
	if _, err := r.createPKI(vc, cv); err != nil {
		return reconcile.Result{}, err
	}
	// 4. deploy etcd
	// 5. deploy apiserver
	// 6. deploy controller-manager

	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualcluster) reconcileVirtualcluster(vc *tenancyv1alpha1.Virtualcluster) (reconcile.Result, error) {
	log.Info("reconciling Virtualcluster(vc)", "vc-name", vc.Name)
	// check if desired clusterversion exists
	cv, exist := r.clusterVersions[vc.Spec.ClusterVersionName]
	if !exist {
		return reconcile.Result{}, fmt.Errorf("desired ClusterVersion %s does not exist", vc.Spec.ClusterVersionName)
	}

	// update exist virtual cluster
	if _, exist := r.virtualClusters[vc.Name]; exist {
		return r.updateVirtualcluster(vc, cv)
	}

	// create new virtual cluster
	return r.createVirtualcluster(vc, cv)
}

func (r *ReconcileVirtualcluster) reconcileClusterVersion(cv *tenancyv1alpha1.ClusterVersion) (reconcile.Result, error) {
	log.Info("reconciling ClusterVersion(cv)", "cv-name", cv.Name)
	log.Info("adding new ClusterVersion(cv)", "cv-name", cv.Name)
	r.clusterVersions[cv.Name] = cv
	return reconcile.Result{}, nil
}

// Reconcile reads that state of the cluster for a Virtualcluster object and makes changes based on the state read
// and what is in the Virtualcluster.Spec
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions/status,verbs=get;update;patch
func (r *ReconcileVirtualcluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("reconciling mainloop...")
	cr, crKind, err := r.checkCRKind(&request)

	switch crKind {
	case virtualCluster:
		// reconcile the Virtualcluster
		vc, ok := cr.(*tenancyv1alpha1.Virtualcluster)
		if !ok {
			return reconcile.Result{}, errors.New("fail to assert Virtualcluste")
		}
		return r.reconcileVirtualcluster(vc)
	case clusterVersion:
		// reconcile the ClusterVersion
		cv, ok := cr.(*tenancyv1alpha1.ClusterVersion)
		if !ok {
			return reconcile.Result{}, errors.New("fail to assert ClusterVersion")
		}
		return r.reconcileClusterVersion(cv)
	default:
		return reconcile.Result{}, err
	}
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
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
