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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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

	tenancyv1alpha1 "github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/controller/kubeconfig"
	vcpki "github.com/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
)

var log = logf.Log.WithName("virtualcluster-controller")

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
	scheme *runtime.Scheme
}

// createNamespace creates a namespace for the virtual cluster. Each
// virtual cluster will have a corresponding namespace whose name
// is same as the virtual cluster's name
func (r *ReconcileVirtualcluster) createNamespace(ns string) error {
	log.Info("creating namespace", "ns", ns)
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	return r.Create(context.TODO(), namespace)
}

// createPKI constructs the PKI (all crt/key pair and kubeconfig) for the
// virtual clusters, and store them as secrets in the meta cluster
func (r *ReconcileVirtualcluster) createPKI(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) error {
	caGroup := &vcpki.ClusterCAGroup{}
	// create root ca, all components will share a single root ca
	rootCAPair, rootCAErr := vcpki.NewCertificateAuthority(&cert.Config{
		CommonName:   "kubernetes",
		Organization: []string{"kubernetes-sig.multi-tenancy.virtualcluster"},
	})
	if rootCAErr != nil {
		return rootCAErr
	}
	caGroup.RootCA = rootCAPair

	etcdDomain := cv.GetEtcdDomain()

	// create crt, key for etcd
	etcdCAPair, etcdCrtErr := vcpki.NewEtcdServerCrtAndKey(rootCAPair, etcdDomain)
	if etcdCrtErr != nil {
		return etcdCrtErr
	}
	caGroup.ETCD = etcdCAPair

	// create crt, key for apiserver
	apiserverDomain := cv.GetAPIServerDomain()
	apiserverCAPair, apiserverCrtErr :=
		vcpki.NewAPIServerCrtAndKey(rootCAPair, vc, apiserverDomain)
	if apiserverCrtErr != nil {
		return apiserverCrtErr
	}
	caGroup.APIServer = apiserverCAPair

	// create kubeconfig for controller-manager
	ctrlmgrKbCfg, cmKbCfgErr := kubeconfig.GenerateKubeconfig(
		"system:kube-controller-manager", vc.Name, rootCAPair, apiserverDomain)
	if cmKbCfgErr != nil {
		return cmKbCfgErr
	}
	caGroup.CtrlMgrKbCfg = ctrlmgrKbCfg

	// create kubeconfig for admin user
	adminKbCfg, adminKbCfgErr := kubeconfig.GenerateKubeconfig(
		"admin", vc.Name, rootCAPair, apiserverDomain)
	if adminKbCfgErr != nil {
		return adminKbCfgErr
	}
	caGroup.AdminKbCfg = adminKbCfg

	// create rsa key for service-account
	svcAcctCAPair, saCrtErr := vcpki.NewServiceAccountSigningKey()
	if saCrtErr != nil {
		return saCrtErr
	}
	caGroup.ServiceAccountPrivateKey = svcAcctCAPair

	// store ca and kubeconfig into secrets
	genSrtsErr := r.createPKISecrets(caGroup, vc.Name)
	if genSrtsErr != nil {
		return genSrtsErr
	}

	return nil
}

// createPKISecrets creates secrets to store crt/key pairs and kubeconfigs
// for master components of the virtual cluster
func (r *ReconcileVirtualcluster) createPKISecrets(caGroup *vcpki.ClusterCAGroup, namespace string) error {
	// create secret for root crt/key pair
	rootSrt, err := secret.CrtKeyPairToSecret(secret.RootCASecretName,
		namespace, caGroup.RootCA)
	if err != nil {
		return err
	}
	// create secret for apiserver crt/key pair
	apiserverSrt, err := secret.CrtKeyPairToSecret(secret.APIServerCASecretName,
		namespace, caGroup.APIServer, caGroup.RootCA.Crt,
		caGroup.ServiceAccountPrivateKey)
	if err != nil {
		return err
	}
	// create secret for etcd crt/key pair
	etcdSrt, err := secret.CrtKeyPairToSecret(secret.ETCDCASecretName,
		namespace, caGroup.ETCD, caGroup.RootCA.Crt)
	if err != nil {
		return err
	}
	// create secret for controller manager kubeconfig
	ctrlMgrSrt := secret.KubeconfigToSecret(secret.ControllerManagerSecretName,
		namespace, caGroup.CtrlMgrKbCfg)
	// create secret for admin kubeconfig
	adminSrt := secret.KubeconfigToSecret(secret.AdminSecretName,
		namespace, caGroup.AdminKbCfg)
	// create secret for service account rsa key
	svcActSrt, err := secret.RsaKeyToSecret(secret.ServiceAccountSecretName,
		namespace, caGroup.ServiceAccountPrivateKey)
	if err != nil {
		return err
	}
	secrets := []*v1.Secret{rootSrt, apiserverSrt, etcdSrt,
		ctrlMgrSrt, adminSrt, svcActSrt}

	// create all secrets on metacluster
	for _, srt := range secrets {
		log.Info("creating secret", "name",
			srt.Name, "namespace", srt.Namespace)
		err := r.Create(context.TODO(), srt)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileVirtualcluster) createVirtualcluster(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (reconcile.Result, error) {
	// 1. create ns
	if err := r.createNamespace(vc.Name); err != nil {
		return reconcile.Result{}, err
	}

	// 2. create pki
	if err := r.createPKI(vc, cv); err != nil {
		return reconcile.Result{}, err
	}

	// 4. deploy etcd
	// err := deployETCD(vc, cv)
	// if err != nil {
	// 	return nil, err
	// }

	// // 5. deploy apiserver
	// err := deployAPIServer(vc, cv)

	// // 6. deploy controller-manager
	// err := deployControllerManager(vc, cv)

	return reconcile.Result{}, nil
}

// Reconcile reads that state of the cluster for a Virtualcluster object and makes changes based on the state read
// and what is in the Virtualcluster.Spec
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=virtualclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenancy.x-k8s.io,resources=clusterversions/status,verbs=get;update;patch
func (r *ReconcileVirtualcluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("reconciling Virtualcluster...")
	vc := &tenancyv1alpha1.Virtualcluster{}
	if err := r.Get(context.TODO(), request.NamespacedName, vc); err != nil {
		return reconcile.Result{}, ignoreNotFound(err)
	}

	cvs := &tenancyv1alpha1.ClusterVersionList{}
	if err := r.List(context.TODO(), cvs, client.InNamespace("")); err != nil {
		return reconcile.Result{}, err
	}

	cv := getClusterVersion(cvs, vc.Spec.ClusterVersionName)
	if cv == nil {
		return reconcile.Result{},
			fmt.Errorf("desired ClusterVersion %s not found",
				vc.Spec.ClusterVersionName)
	}

	return r.createVirtualcluster(vc, cv)
}

func getClusterVersion(cvl *tenancyv1alpha1.ClusterVersionList, cvn string) *tenancyv1alpha1.ClusterVersion {
	for _, cv := range cvl.Items {
		if cv.Name == cvn {
			return &cv
		}
	}
	return nil
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
