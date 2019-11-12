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
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/kubeconfig"
	vcpki "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
)

const (
	DefaultETCDPeerPort = 2380

	// frequency of polling apiserver for readiness of each component
	ComponentPollPeriod = 2 * time.Second
	// timeout for components deployment
	DeployTimeOut = 120 * time.Second
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
	return &ReconcileVirtualcluster{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualcluster-controller",
		mgr, controller.Options{Reconciler: r})
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
}

// createPKI constructs the PKI (all crt/key pair and kubeconfig) for the
// virtual clusters, and store them as secrets in the meta cluster
func (r *ReconcileVirtualcluster) createPKI(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) error {
	caGroup := &vcpki.ClusterCAGroup{}
	// create root ca, all components will share a single root ca
	rootCACrt, rootKey, rootCAErr := pkiutil.NewCertificateAuthority(
		&cert.Config{
			CommonName:   "kubernetes",
			Organization: []string{"kubernetes-sig.kubernetes-sigs/multi-tenancy.virtualcluster"},
		})
	if rootCAErr != nil {
		return rootCAErr
	}

	rootRsaKey, ok := rootKey.(*rsa.PrivateKey)
	if !ok {
		return errors.New("fail to assert rsa PrivateKey")
	}

	rootCAPair := &vcpki.CrtKeyPair{
		Crt: rootCACrt,
		Key: rootRsaKey,
	}
	caGroup.RootCA = rootCAPair

	etcdDomains := append(cv.GetEtcdServers(), cv.GetEtcdDomain())
	// create crt, key for etcd
	etcdCAPair, etcdCrtErr := vcpki.NewEtcdServerCrtAndKey(rootCAPair, etcdDomains)
	if etcdCrtErr != nil {
		return etcdCrtErr
	}
	caGroup.ETCD = etcdCAPair

	// create crt, key for apiserver
	apiserverDomain := cv.GetAPIServerDomain(vc.Namespace)
	apiserverCAPair, apiserverCrtErr :=
		vcpki.NewAPIServerCrtAndKey(rootCAPair, vc, apiserverDomain)
	if apiserverCrtErr != nil {
		return apiserverCrtErr
	}
	caGroup.APIServer = apiserverCAPair

	// create kubeconfig for controller-manager
	ctrlmgrKbCfg, cmKbCfgErr := kubeconfig.GenerateKubeconfig(
		"system:kube-controller-manager",
		vc.Name, apiserverDomain, []string{}, rootCAPair)
	if cmKbCfgErr != nil {
		return cmKbCfgErr
	}
	caGroup.CtrlMgrKbCfg = ctrlmgrKbCfg

	// create kubeconfig for admin user
	adminKbCfg, adminKbCfgErr := kubeconfig.GenerateKubeconfig(
		"admin", vc.Name, apiserverDomain,
		[]string{"system:masters"}, rootCAPair)
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
	genSrtsErr := r.createPKISecrets(caGroup, vc.Namespace)
	if genSrtsErr != nil {
		return genSrtsErr
	}

	return nil
}

// genInitialClusterArgs generates the values for `--inital-cluster` option of etcd based on the number of
// replicas specified in etcd StatefulSet
func genInitialClusterArgs(replicas int32, stsName, svcName string) (argsVal string) {
	for i := int32(0); i < replicas; i++ {
		// use 2380 as the default port for etcd peer communication
		peerAddr := fmt.Sprintf("%s-%d=https://%s-%d.%s:%d",
			stsName, i, stsName, i, svcName, DefaultETCDPeerPort)
		if i == replicas-1 {
			argsVal = argsVal + peerAddr
			break
		}
		argsVal = argsVal + peerAddr + ","
	}

	return argsVal
}

// completmentETCDTemplate completments the ETCD template of the specified clusterversion
// based on the virtual cluster setting
func completmentETCDTemplate(vcns string, etcdBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	etcdBdl.StatefulSet.ObjectMeta.Namespace = vcns
	etcdBdl.Service.ObjectMeta.Namespace = vcns
	args := etcdBdl.StatefulSet.Spec.Template.Spec.Containers[0].Args
	icaVal := genInitialClusterArgs(*etcdBdl.StatefulSet.Spec.Replicas,
		etcdBdl.StatefulSet.Name, etcdBdl.Service.Name)
	args = append(args, "--initial-cluster", icaVal)
	etcdBdl.StatefulSet.Spec.Template.Spec.Containers[0].Args = args
}

// completmentAPIServerTemplate completments the apiserver template of the specified clusterversion
// based on the virtual cluster setting
func completmentAPIServerTemplate(vcns string, apiserverBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	apiserverBdl.StatefulSet.ObjectMeta.Namespace = vcns
	apiserverBdl.Service.ObjectMeta.Namespace = vcns
}

// completmentCtrlMgrTemplate completments the controller manager template of the specified clusterversion
// based on the virtual cluster setting
func completmentCtrlMgrTemplate(vcns string, ctrlMgrBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	ctrlMgrBdl.StatefulSet.ObjectMeta.Namespace = vcns
}

// deployComponent deploys master component in namespace vcName based on the given StatefulSet
// and Service Bundle ssBdl
func (r *ReconcileVirtualcluster) deployComponent(vc *tenancyv1alpha1.Virtualcluster, ssBdl *tenancyv1alpha1.StatefulSetSvcBundle) error {
	log.Info("deploying StatefulSet for master component", "component",
		ssBdl.Name)

	switch ssBdl.Name {
	case "etcd":
		completmentETCDTemplate(vc.Namespace, ssBdl)
	case "apiserver":
		completmentAPIServerTemplate(vc.Namespace, ssBdl)
	case "controller-manager":
		completmentCtrlMgrTemplate(vc.Namespace, ssBdl)
	default:
		return fmt.Errorf("try to deploy unknwon component: %s", ssBdl.Name)
	}

	err := r.Create(context.TODO(), ssBdl.StatefulSet)
	if err != nil {
		return err
	}

	err = controllerutil.SetControllerReference(vc, ssBdl.StatefulSet, r.scheme)
	if err != nil {
		return err
	}

	if ssBdl.Service != nil {
		log.Info("deploying Service for master component", "component",
			ssBdl.Name)
		err = r.Create(context.TODO(), ssBdl.Service)
		if err != nil {
			return err
		}
		err := controllerutil.SetControllerReference(vc, ssBdl.Service, r.scheme)
		if err != nil {
			return err
		}
	}

	// make sure the StatsfulSet is ready
	// (i.e. Status.ReadyReplicas == Spec.Replicas)
	timeout := time.After(DeployTimeOut)
	for {
		period := time.After(ComponentPollPeriod)
		select {
		case <-timeout:
			return fmt.Errorf("deploy %s timeout", ssBdl.Name)
		case <-period:
			sts := &appsv1.StatefulSet{}
			if err := r.Get(context.TODO(), types.NamespacedName{
				Namespace: vc.Namespace,
				Name:      ssBdl.Name},
				sts); err != nil {
				return err
			}
			if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
				log.Info("component is ready", "component", ssBdl.Name)
				return nil
			}
		}
	}

}

// createPKISecrets creates secrets to store crt/key pairs and kubeconfigs
// for master components of the virtual cluster
func (r *ReconcileVirtualcluster) createPKISecrets(caGroup *vcpki.ClusterCAGroup, namespace string) error {
	// create secret for root crt/key pair
	rootSrt := secret.CrtKeyPairToSecret(secret.RootCASecretName,
		namespace, caGroup.RootCA)
	// create secret for apiserver crt/key pair
	apiserverSrt := secret.CrtKeyPairToSecret(secret.APIServerCASecretName,
		namespace, caGroup.APIServer)
	// create secret for etcd crt/key pair
	etcdSrt := secret.CrtKeyPairToSecret(secret.ETCDCASecretName,
		namespace, caGroup.ETCD)
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

// createVirtualcluster creates a new Virtualcluster based on the specified ClusterVersion
func (r *ReconcileVirtualcluster) createVirtualcluster(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (err error) {
	defer func() {
		if err != nil {
			log.Error(err,
				"fail to create virtualcluster, remove namespace for deleting related resources")
		}
	}()

	// 1. create PKI
	err = r.createPKI(vc, cv)
	if err != nil {
		return
	}

	// 2. deploy etcd
	err = r.deployComponent(vc, cv.Spec.ETCD)
	if err != nil {
		return
	}

	// 3. deploy apiserver
	err = r.deployComponent(vc, cv.Spec.APIServer)
	if err != nil {
		return
	}

	// 4. deploy controller-manager
	err = r.deployComponent(vc, cv.Spec.ControllerManager)
	if err != nil {
		return
	}

	return
}

// Reconcile reads that state of the cluster for a Virtualcluster object and makes changes based on the state read
// and what is in the Virtualcluster.Spec
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
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

	// TODO implement the delete logic (finalizer)

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
		cvs := &tenancyv1alpha1.ClusterVersionList{}
		err = r.List(context.TODO(), cvs, client.InNamespace(""))
		if err != nil {
			return
		}

		cv := getClusterVersion(cvs, vc.Spec.ClusterVersionName)
		if cv == nil {
			err = fmt.Errorf("desired ClusterVersion %s not found",
				vc.Spec.ClusterVersionName)
			return

		}
		err = r.createVirtualcluster(vc, cv)
		if err != nil {
			return
		}
		// all components are ready, update vc status
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			vc.Status.Phase = "Running"
			vc.Status.Message = "tenant master is running"
			vc.Status.Reason = "TenantMasterRunning"
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

func getClusterVersion(cvl *tenancyv1alpha1.ClusterVersionList, cvn string) *tenancyv1alpha1.ClusterVersion {
	for _, cv := range cvl.Items {
		if cv.Name == cvn {
			return &cv
		}
	}
	return nil
}
