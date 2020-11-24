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

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/kubeconfig"
	vcpki "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	pkiutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/pki"
)

const (
	DefaultETCDPeerPort    = 2380
	ComponentPollPeriodSec = 2
	// timeout for components deployment
	DeployTimeOutSec = 180
)

type MasterProvisionerNative struct {
	client.Client
	scheme *runtime.Scheme
}

func NewMasterProvisionerNative(mgr manager.Manager) (*MasterProvisionerNative, error) {
	return &MasterProvisionerNative{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}, nil
}

// CreateVirtualCluster sets up the control plane for vc on meta k8s
func (mpn *MasterProvisionerNative) CreateVirtualCluster(vc *tenancyv1alpha1.VirtualCluster) error {
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
	// 1. create the root ns
	_, err = kubeutil.CreateRootNS(mpn, vc)
	if err != nil {
		return err
	}
	isClusterIP := cv.Spec.APIServer.Service != nil && cv.Spec.APIServer.Service.Spec.Type == v1.ServiceTypeClusterIP
	// if ClusterIP, have to create API Server ahead of time to lay it down in the PKI
	if isClusterIP {
		log.Info("deploying ClusterIP Service for API component", "component", cv.Spec.APIServer.Name)
		complementAPIServerTemplate(conversion.ToClusterKey(vc), cv.Spec.APIServer)
		err = mpn.Create(context.TODO(), cv.Spec.APIServer.Service)
		if err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			log.Info("service already exist",
				"service", cv.Spec.APIServer.Service.GetName())
		}
	}

	// 2. create PKI
	err = mpn.createPKI(vc, cv, isClusterIP)
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

// complementETCDTemplate complements the ETCD template of the specified clusterversion
// based on the virtual cluster setting
func complementETCDTemplate(vcns string, etcdBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	etcdBdl.StatefulSet.ObjectMeta.Namespace = vcns
	etcdBdl.Service.ObjectMeta.Namespace = vcns
	args := etcdBdl.StatefulSet.Spec.Template.Spec.Containers[0].Args
	icaVal := genInitialClusterArgs(*etcdBdl.StatefulSet.Spec.Replicas,
		etcdBdl.StatefulSet.Name, etcdBdl.Service.Name)
	args = append(args, "--initial-cluster", icaVal)
	etcdBdl.StatefulSet.Spec.Template.Spec.Containers[0].Args = args
}

// complementAPIServerTemplate complements the apiserver template of the specified clusterversion
// based on the virtual cluster setting
func complementAPIServerTemplate(vcns string, apiserverBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	apiserverBdl.StatefulSet.ObjectMeta.Namespace = vcns
	apiserverBdl.Service.ObjectMeta.Namespace = vcns
}

// complementCtrlMgrTemplate complements the controller manager template of the specified clusterversion
// based on the virtual cluster setting
func complementCtrlMgrTemplate(vcns string, ctrlMgrBdl *tenancyv1alpha1.StatefulSetSvcBundle) {
	ctrlMgrBdl.StatefulSet.ObjectMeta.Namespace = vcns
}

// deployComponent deploys master component in namespace vcName based on the given StatefulSet
// and Service Bundle ssBdl
func (mpn *MasterProvisionerNative) deployComponent(vc *tenancyv1alpha1.VirtualCluster, ssBdl *tenancyv1alpha1.StatefulSetSvcBundle) error {
	log.Info("deploying StatefulSet for master component", "component", ssBdl.Name)

	ns := conversion.ToClusterKey(vc)

	switch ssBdl.Name {
	case "etcd":
		complementETCDTemplate(ns, ssBdl)
	case "apiserver":
		complementAPIServerTemplate(ns, ssBdl)
	case "controller-manager":
		complementCtrlMgrTemplate(ns, ssBdl)
	default:
		return fmt.Errorf("try to deploy unknwon component: %s", ssBdl.Name)
	}

	err := mpn.Create(context.TODO(), ssBdl.StatefulSet)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.Info("statefuleset already exist",
			"statefuleset", ssBdl.StatefulSet.GetName(),
			"namespace", ssBdl.StatefulSet.GetNamespace())
	}

	if ssBdl.Service != nil {
		log.Info("deploying Service for master component", "component", ssBdl.Name)
		err = mpn.Create(context.TODO(), ssBdl.Service)
		if err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			log.Info("service already exist",
				"service", ssBdl.Service.GetName())
		}
	}

	// wait for the statefuleset to be ready
	err = kubeutil.WaitStatefulSetReady(mpn, ns, ssBdl.GetName(), DeployTimeOutSec, ComponentPollPeriodSec)
	if err != nil {
		return err
	}
	return nil
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
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			log.Info("Secret already exists",
				"secret", srt.Name,
				"namespace", srt.Namespace)
		}
	}

	return nil
}

// createPKI constructs the PKI (all crt/key pair and kubeconfig) for the
// virtual clusters, and store them as secrets in the meta cluster
func (mpn *MasterProvisionerNative) createPKI(vc *tenancyv1alpha1.VirtualCluster, cv *tenancyv1alpha1.ClusterVersion, isClusterIP bool) error {
	ns := conversion.ToClusterKey(vc)
	caGroup := &vcpki.ClusterCAGroup{}
	// create root ca, all components will share a single root ca
	rootCACrt, rootKey, rootCAErr := pkiutil.NewCertificateAuthority(
		&pkiutil.CertConfig{
			Config: cert.Config{
				CommonName:   "kubernetes",
				Organization: []string{"kubernetes-sig.kubernetes-sigs/multi-tenancy.virtualcluster"},
			},
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

	clusterIP := ""
	if isClusterIP {
		var err error
		clusterIP, err = kubeutil.GetSvcClusterIP(mpn, conversion.ToClusterKey(vc), cv.Spec.APIServer.Service.GetName())
		if err != nil {
			log.Info("Warning: failed to get API Service", "service", cv.Spec.APIServer.Service.GetName(), "err", err)
		}
	}

	apiserverDomain := cv.GetAPIServerDomain(ns)
	apiserverCAPair, err := vcpki.NewAPIServerCrtAndKey(rootCAPair, vc, apiserverDomain, clusterIP)
	if err != nil {
		return err
	}
	caGroup.APIServer = apiserverCAPair

	finalAPIAddress := apiserverDomain
	if clusterIP != "" {
		finalAPIAddress = clusterIP
	}

	// create kubeconfig for controller-manager
	ctrlmgrKbCfg, err := kubeconfig.GenerateKubeconfig(
		"system:kube-controller-manager",
		vc.Name, finalAPIAddress, []string{}, rootCAPair)
	if err != nil {
		return err
	}
	caGroup.CtrlMgrKbCfg = ctrlmgrKbCfg

	// create kubeconfig for admin user
	adminKbCfg, err := kubeconfig.GenerateKubeconfig(
		"admin", vc.Name, finalAPIAddress,
		[]string{"system:masters"}, rootCAPair)
	if err != nil {
		return err
	}
	caGroup.AdminKbCfg = adminKbCfg

	// create rsa key for service-account
	svcAcctCAPair, err := vcpki.NewServiceAccountSigningKey()
	if err != nil {
		return err
	}
	caGroup.ServiceAccountPrivateKey = svcAcctCAPair

	// store ca and kubeconfig into secrets
	genSrtsErr := mpn.createPKISecrets(caGroup, ns)
	if genSrtsErr != nil {
		return genSrtsErr
	}

	return nil
}

func (mpn *MasterProvisionerNative) DeleteVirtualCluster(vc *tenancyv1alpha1.VirtualCluster) error {
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
