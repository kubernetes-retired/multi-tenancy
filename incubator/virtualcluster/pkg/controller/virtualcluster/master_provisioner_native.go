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
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/kubeconfig"
	vcpki "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	WaitKubeSvcTimeOutSec = 120
	DefaultETCDPeerPort   = 2380

	// frequency of polling apiserver for readiness of each component
	ComponentPollPeriodSec = 2
	// timeout for components deployment
	DeployTimeOutSec = 180
	// wait the apiserver reboot timeout
	APIServerRebootTimeOutSec = 240
)

type MasterProvisionerNative struct {
	client.Client
	scheme *runtime.Scheme
}

func NewMasterProvisionerNative(mgr manager.Manager) *MasterProvisionerNative {
	return &MasterProvisionerNative{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// CreateVirtualCluster sets up the control plane for vc on meta k8s
func (mpn *MasterProvisionerNative) CreateVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	cvs := &tenancyv1alpha1.ClusterVersionList{}
	err := mpn.List(context.TODO(), cvs, client.InNamespace(""))
	if err != nil {
		return err
	}

	cv := getClusterVersion(cvs, vc.Spec.ClusterVersionName)
	if cv == nil {
		err = fmt.Errorf("desired ClusterVersion %s not found",
			vc.Spec.ClusterVersionName)
		return err
	}
	defer func() {
		if err != nil {
			log.Error(err, "fail to create virtualcluster, remove namespace for deleting related resources")
		}
	}()

	// 1. create ns
	err = mpn.createNS(vc)
	if err != nil {
		return err
	}

	// 2. create PKI
	caGroup, err := mpn.createPKI(vc, cv)
	if err != nil {
		return err
	}

	// 3. deploy etcd
	err = mpn.deployComponent(vc, cv.Spec.ETCD)
	if err != nil {
		return err
	}

	// 4. deploy apiserver
	err = mpn.deployComponent(vc, cv.Spec.APIServer)
	if err != nil {
		return err
	}

	// 5. deploy controller-manager
	err = mpn.deployComponent(vc, cv.Spec.ControllerManager)
	if err != nil {
		return err
	}

	// 6. update certificate once kubernetes service has been synced to super master
	certUpdated := make(chan struct{})
	go mpn.updateAPIServerCrtAndKey(caGroup, vc, cv, certUpdated)

	// 7. reboot the apiserver to load the latest certificate
	go mpn.rebootAPIServer(vc, cv, certUpdated)

	return nil
}

// rebootAPISrver reboots the apiserver pod of Virtualcluster 'vc'
func (mpn *MasterProvisionerNative) rebootAPIServer(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion, certUpdated <-chan struct{}) error {
	<-certUpdated
	log.Info("restarting apiserver for reloading certificate", "vc", vc.GetName())
	// change vc status to "Updating"
	if err := mpn.updateVcStatus(vc,
		tenancyv1alpha1.ClusterUpdating,
		"restarting the apiserver",
		"reload the new certificate"); err != nil {
		return err
	}

	// delete the pod
	p := &v1.Pod{}
	if err := mpn.Get(context.TODO(), types.NamespacedName{
		Namespace: conversion.ToClusterKey(vc),
		Name:      "apiserver-0",
	}, p); err != nil {
		log.Error(err, "vc", vc.GetName())
		return err
	}

	if err := mpn.Delete(context.TODO(), p); err != nil {
		log.Error(err, "vc", vc.GetName())
		return err
	}
	log.Info("delete the old apiserver pod for the Virtualcluster", "vc-name", vc.GetName())

	// wait for statefulset controller to restart the pod
	if err := mpn.pollStatefulSet(conversion.ToClusterKey(vc),
		"apiserver",
		vc.GetName(),
		APIServerRebootTimeOutSec,
		ComponentPollPeriodSec); err != nil {
		return err
	}

	// change the VC status to "Ready"
	log.Info("apiserver sts is ready")
	if err := mpn.updateVcStatus(vc,
		tenancyv1alpha1.ClusterReady,
		"tenant master is ready",
		"apiserver certificate has been updated"); err != nil {
		return err
	}
	return nil
}

// updateVcStatus updates the status of Virtualcluster 'vc'
func (mpn *MasterProvisionerNative) updateVcStatus(vc *tenancyv1alpha1.Virtualcluster, phase tenancyv1alpha1.ClusterPhase, message, reason string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		vc.Status.Phase = tenancyv1alpha1.ClusterRunning
		vc.Status.Message = "tenant master is running"
		vc.Status.Reason = "TenantMasterRunning"
		updateErr := mpn.Update(context.TODO(), vc)
		if err := mpn.Get(context.TODO(), types.NamespacedName{
			Namespace: vc.GetNamespace(),
			Name:      vc.GetName(),
		}, vc); err != nil {
			log.Info("fail to get vc on update failure", "error", err, "vc", vc.GetName())
		}
		return updateErr
	})
}

// pollStatefulSet polls the status of the statefulset 'namespace/name'.
// It returns nil if the ReadyReplicas equals to the the Replicas within the 'timeoutSec'
func (mpn *MasterProvisionerNative) pollStatefulSet(namespace, name, vcName string, timeoutSec, periodSec int64) error {
	timeOut := time.After(time.Duration(timeoutSec) * time.Second)
	stsFullPath := namespace + "/" + name
PollStatefulSet:
	for {
		select {
		case <-time.After(time.Duration(periodSec) * time.Second):
			log.Info("polling statefulset", "statefulset", stsFullPath, "vc", vcName)
			sts := &appsv1.StatefulSet{}
			if err := mpn.Get(context.TODO(), types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			}, sts); err != nil {
				log.Info("fail to get statefulset", "statefulset", stsFullPath, "vc", vcName)
				return err
			}
			if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
				log.Info("statefulset is ready", "statefulset", stsFullPath, "vc", vcName)
				break PollStatefulSet
			}
		case <-timeOut:
			log.Info("fail to poll statefulset", "statefulset", stsFullPath, "vc", vcName)
			return fmt.Errorf("fail to poll statefulset", "statefulset", stsFullPath, "vc", vcName)
		}
	}
	return nil
}

// updateAPIServerCrtAndKey updates the tenant apiserver's certificate and key
// by adding the clusterIP of kubernetes service's
func (mpn *MasterProvisionerNative) updateAPIServerCrtAndKey(caGroup *vcpki.ClusterCAGroup, vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion, certUpdated chan<- struct{}) error {
	// get the clusterIP of the corresponding kubernetes service on super master
	waitForKubeSvcTimeOut := time.After(WaitKubeSvcTimeOutSec * time.Second)
	svc := &v1.Service{}
PollKubeSvc:
	for {
		select {
		case <-time.After(10 * time.Second):
			svcNs := conversion.
				ToSuperMasterNamespace(conversion.ToClusterKey(vc), "default")
			err := mpn.Get(context.TODO(), types.NamespacedName{
				Namespace: svcNs,
				Name:      "kubernetes"}, svc)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue PollKubeSvc
				}
				return err
			}
			log.Info("kubernetes service is ready", "namespace", svcNs)
			break PollKubeSvc
		case <-waitForKubeSvcTimeOut:
			return errors.New("wait for kuberentes service time out")
		}
	}

	kubeClusterIP := svc.Spec.ClusterIP
	log.Info("updaing apiserver certificate with kubernetes clusterIP",
		"clusterIP", kubeClusterIP)

	// regenerate cert and key based on clusterIP and podIP
	log.Info("updating apiserver certificate by adding default/kubernetes clusterIP", "vc", vc.GetName(), "clusterIP", kubeClusterIP)
	ns := conversion.ToClusterKey(vc)
	apiserverDomain := cv.GetAPIServerDomain(ns)
	apiserverCAPair, err := vcpki.NewAPIServerCrtAndKey(caGroup.RootCA,
		vc, apiserverDomain, kubeClusterIP)
	if err != nil {
		return err
	}

	// update the caGroup
	caGroup.APIServer = apiserverCAPair

	// update the corresponding secretes
	apiserverSrt := &v1.Secret{}
	err = mpn.Get(context.TODO(), types.NamespacedName{
		Namespace: conversion.ToClusterKey(vc),
		Name:      secret.APIServerCASecretName}, apiserverSrt)
	if err != nil {
		return err
	}
	apiserverSrt.Data[v1.TLSCertKey] =
		pkiutil.EncodeCertPEM(apiserverCAPair.Crt)
	apiserverSrt.Data[v1.TLSPrivateKeyKey] =
		vcpki.EncodePrivateKeyPEM(apiserverCAPair.Key)
	err = mpn.Update(context.TODO(), apiserverSrt)
	if err != nil {
		return err
	}
	log.Info("the secret of apiserver certificate has been updated", "vc", vc.GetName())

	close(certUpdated)

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
func (mpn *MasterProvisionerNative) deployComponent(vc *tenancyv1alpha1.Virtualcluster, ssBdl *tenancyv1alpha1.StatefulSetSvcBundle) error {
	log.Info("deploying StatefulSet for master component", "component", ssBdl.Name)

	ns := conversion.ToClusterKey(vc)

	switch ssBdl.Name {
	case "etcd":
		completmentETCDTemplate(ns, ssBdl)
	case "apiserver":
		completmentAPIServerTemplate(ns, ssBdl)
	case "controller-manager":
		completmentCtrlMgrTemplate(ns, ssBdl)
	default:
		return fmt.Errorf("try to deploy unknwon component: %s", ssBdl.Name)
	}

	err := mpn.Create(context.TODO(), ssBdl.StatefulSet)
	if err != nil {
		return err
	}

	err = controllerutil.SetControllerReference(vc, ssBdl.StatefulSet, mpn.scheme)
	if err != nil {
		return err
	}

	if ssBdl.Service != nil {
		log.Info("deploying Service for master component", "component", ssBdl.Name)
		err = mpn.Create(context.TODO(), ssBdl.Service)
		if err != nil {
			return err
		}
		err = controllerutil.SetControllerReference(vc, ssBdl.Service, mpn.scheme)
		if err != nil {
			return err
		}
	}

	// make sure the StatsfulSet is ready
	// (i.e. Status.ReadyReplicas == Spec.Replicas)
	err = mpn.pollStatefulSet(ns, ssBdl.Name, vc.GetName(), DeployTimeOutSec, ComponentPollPeriodSec)
	return err
}

// createPKISecrets creates secrets to store crt/key pairs and kubeconfigs
// for master components of the virtual cluster
func (mpn *MasterProvisionerNative) createPKISecrets(caGroup *vcpki.ClusterCAGroup, namespace string) error {
	// create secret for root crt/key pair
	rootSrt := secret.CrtKeyPairToSecret(secret.RootCASecretName, namespace, caGroup.RootCA)
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
		err := mpn.Create(context.TODO(), srt)
		if err != nil {
			return err
		}
	}

	return nil
}

func (mpn *MasterProvisionerNative) createNS(vc *tenancyv1alpha1.Virtualcluster) error {
	err := mpn.Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: conversion.ToClusterKey(vc),
		},
	})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// createPKI constructs the PKI (all crt/key pair and kubeconfig) for the
// virtual clusters, and store them as secrets in the meta cluster
func (mpn *MasterProvisionerNative) createPKI(vc *tenancyv1alpha1.Virtualcluster, cv *tenancyv1alpha1.ClusterVersion) (*vcpki.ClusterCAGroup, error) {
	ns := conversion.ToClusterKey(vc)
	caGroup := &vcpki.ClusterCAGroup{}
	// create root ca, all components will share a single root ca
	rootCACrt, rootKey, rootCAErr := pkiutil.NewCertificateAuthority(
		&cert.Config{
			CommonName:   "kubernetes",
			Organization: []string{"kubernetes-sig.kubernetes-sigs/multi-tenancy.virtualcluster"},
		})
	if rootCAErr != nil {
		return nil, rootCAErr
	}

	rootRsaKey, ok := rootKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fail to assert rsa PrivateKey")
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
		return nil, etcdCrtErr
	}
	caGroup.ETCD = etcdCAPair

	// create crt, key for apiserver
	apiserverDomain := cv.GetAPIServerDomain(ns)
	apiserverCAPair, err := vcpki.NewAPIServerCrtAndKey(rootCAPair, vc, apiserverDomain)
	if err != nil {
		return nil, err
	}
	caGroup.APIServer = apiserverCAPair

	// create kubeconfig for controller-manager
	ctrlmgrKbCfg, err := kubeconfig.GenerateKubeconfig(
		"system:kube-controller-manager",
		vc.Name, apiserverDomain, []string{}, rootCAPair)
	if err != nil {
		return nil, err
	}
	caGroup.CtrlMgrKbCfg = ctrlmgrKbCfg

	// create kubeconfig for admin user
	adminKbCfg, err := kubeconfig.GenerateKubeconfig(
		"admin", vc.Name, apiserverDomain,
		[]string{"system:masters"}, rootCAPair)
	if err != nil {
		return nil, err
	}
	caGroup.AdminKbCfg = adminKbCfg

	// create rsa key for service-account
	svcAcctCAPair, err := vcpki.NewServiceAccountSigningKey()
	if err != nil {
		return nil, err
	}
	caGroup.ServiceAccountPrivateKey = svcAcctCAPair

	// store ca and kubeconfig into secrets
	genSrtsErr := mpn.createPKISecrets(caGroup, ns)
	if genSrtsErr != nil {
		return nil, genSrtsErr
	}

	return caGroup, nil
}

func (mpn *MasterProvisionerNative) DeleteVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	return nil
}

func (mpn *MasterProvisionerNative) GetMasterProvisioner() string {
	return "native"
}

func getClusterVersion(cvl *tenancyv1alpha1.ClusterVersionList, cvn string) *tenancyv1alpha1.ClusterVersion {
	for _, cv := range cvl.Items {
		if cv.Name == cvn {
			return &cv
		}
	}
	return nil
}
