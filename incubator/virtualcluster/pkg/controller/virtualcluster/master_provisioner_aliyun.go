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
	"encoding/base64"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
	aliyunutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/aliyun"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
)

type MasterProvisionerAliyun struct {
	client.Client
	scheme *runtime.Scheme
}

func NewMasterProvisionerAliyun(mgr manager.Manager) (*MasterProvisionerAliyun, error) {
	// if running under aliyun mode, 'AliyunAkSrt' and 'AliyunASKConfigMap' is required
	ns, err := kubeutil.GetPodNsFromInside()
	if err != nil {
		return nil, fmt.Errorf("fail to get vc-manager's namespace: %s", err)
	}
	cli, err := kubeutil.NewInClusterClient()
	if err != nil {
		return nil, fmt.Errorf("fail to create incluster client: %s", err)
	}
	if !kubeutil.IsObjExist(cli, types.NamespacedName{
		Namespace: ns,
		Name:      aliyunutil.AliyunAkSrt,
	}, &v1.Secret{}, log) {
		return nil, fmt.Errorf("secret/%s doesnot exist", aliyunutil.AliyunAkSrt)
	}
	if !kubeutil.IsObjExist(cli, types.NamespacedName{
		Namespace: ns,
		Name:      aliyunutil.AliyunASKConfigMap,
	}, &v1.ConfigMap{}, log) {
		return nil, fmt.Errorf("configmap/%s doesnot exist", aliyunutil.AliyunASKConfigMap)
	}
	return &MasterProvisionerAliyun{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}, nil
}

// CreateVirtualCluster creates a new ASK on aliyun for given VirtualCluster
func (mpa *MasterProvisionerAliyun) CreateVirtualCluster(vc *tenancyv1alpha1.VirtualCluster) error {
	log.Info("setting up control plane for the VirtualCluster", "VirtualCluster", vc.Name)
	// 1. load aliyun accessKeyID/accessKeySecret from secret
	aliyunAKID, aliyunAKSrt, err := aliyunutil.GetAliyunAKPair(mpa, log)
	if err != nil {
		return err
	}

	askCfg, err := aliyunutil.GetASKConfigs(mpa, log)
	if err != nil {
		return err
	}

	// 2. send ASK creation request
	// NOTE http requests of a creation action will be sent by a same client
	cli, err := sdk.NewClientWithAccessKey(askCfg.RegionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	var (
		clsID    string
		clsState string
		clsSlbId string
	)
	creationTimeout := time.After(600 * time.Second)
	clsID, err = aliyunutil.SendCreationRequest(cli, vc.Name, askCfg)
	if err != nil {
		if !aliyunutil.IsSDKErr(err) {
			return err
		}
		// check SDK error code
		if aliyunutil.GetSDKErrCode(err) == aliyunutil.ClusterNameAlreadyExist {
			// clusterName already exists, query Aliyun to get the clusterID
			// corresponding to the clusterName
			var getClsIDErr error
			clsID, getClsIDErr = aliyunutil.GetClusterIDByName(cli, vc.Name, askCfg.RegionID)
			if getClsIDErr != nil {
				return getClsIDErr
			}
			var getStErr error
			clsSlbId, clsState, getStErr = aliyunutil.GetASKStateAndSlbID(cli, clsID, askCfg.RegionID)
			if getStErr != nil {
				return getStErr
			}

			if clsState != "running" && clsState != "initial" {
				return fmt.Errorf("unknown ASK(%s) state: %s", vc.Name, clsState)
			}
		}
	}

	log.Info("creating the ASK", "ASK-ID", clsID)

	// 3. block until the newly created ASK is up and running
PollASK:
	for {
		select {
		case <-time.After(10 * time.Second):
			if clsState == "running" {
				// ASK is up and running, stop polling
				log.Info("ASK is up and running", "ASK-ID", clsID)
				break PollASK
			}
			var getStErr error
			clsSlbId, clsState, getStErr = aliyunutil.GetASKStateAndSlbID(cli, clsID, askCfg.RegionID)
			if getStErr != nil {
				return getStErr
			}

		case <-creationTimeout:
			return fmt.Errorf("creating cluster(%s) timeout", clsID)
		}
	}

	// 4. create the root namesapce of the VirtualCluster
	vcNs, err := kubeutil.CreateRootNS(mpa, vc)
	if err != nil {
		return err
	}
	log.Info("virtualcluster root ns is created", "ns", vcNs)

	// 5. get kubeconfig of the newly created ASK
	kbCfg, err := aliyunutil.GetASKKubeConfig(cli, clsID, askCfg.RegionID, askCfg.PrivateKbCfg)
	if err != nil {
		return err
	}
	log.Info("got kubeconfig of cluster", "cluster", clsID)

	// 6. serialize kubeconfig to secret and store it in the
	// corresponding namespace (i.e.)
	adminSrt := secret.KubeconfigToSecret(secret.AdminSecretName,
		vcNs, kbCfg)
	err = mpa.Create(context.TODO(), adminSrt)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	log.Info("admin kubeconfig is created for virtualcluster", "vc", vc.GetName())

	// 7. add annotations on vc cr, including,
	// tenancy.x-k8s.io/ask.clusterID,
	// tenancy.x-k8s.io/ask.slbID,
	// tenancy.x-k8s.io/cluster,
	// tenancy.x-k8s.io/admin-kubeconfig
	err = kubeutil.AnnotateVC(mpa, vc, aliyunutil.AnnotationSlbID, clsSlbId, log)
	if err != nil {
		return err
	}
	log.Info("slb id has been added to vc as an annotation", "vc", vc.GetName(), "id", clsSlbId)
	err = kubeutil.AnnotateVC(mpa, vc, aliyunutil.AnnotationClusterID, clsID, log)
	if err != nil {
		return err
	}
	log.Info("cluster ID has been added to vc as an annotation", "vc", vc.GetName(), "cluster-id", clsID)
	kbCfgB64 := base64.StdEncoding.EncodeToString([]byte(kbCfg))
	err = kubeutil.AnnotateVC(mpa, vc, aliyunutil.AnnotationKubeconfig, kbCfgB64, log)
	if err != nil {
		return err
	}
	log.Info("admin-kubeconfig has been added to vc as an annotation", "vc", vc.GetName())
	// the clusterkey is the vc root ns
	err = kubeutil.AnnotateVC(mpa, vc, constants.LabelCluster, vcNs, log)
	if err != nil {
		return err
	}
	log.Info("cluster key has been added to vc as an annotation", "vc", vc.GetName(), "cluster-key", vcNs)

	// 8. delete the node-controller service account to disable the
	// node periodic check
	tenantCliCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kbCfg))
	if err != nil {
		return err
	}
	tenantCli, err := kubernetes.NewForConfig(tenantCliCfg)
	if err != nil {
		return err
	}
	// delete if exist
	if err = tenantCli.CoreV1().ServiceAccounts("kube-system").
		Delete("node-controller", &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	log.Info("the node selector service account is deleted", "vc", vc.GetName())
	return nil
}

// DeleteVirtualCluster deletes the ASK cluster corresponding to the given VirtualCluster
// NOTE DeleteVirtualCluster only sends the deletion request to Aliyun and do not promise
// the ASK will be deleted
func (mpa *MasterProvisionerAliyun) DeleteVirtualCluster(vc *tenancyv1alpha1.VirtualCluster) error {
	log.Info("deleting the ASK of the virtualcluster", "vc-name", vc.Name)
	aliyunAKID, aliyunAKSrt, err := aliyunutil.GetAliyunAKPair(mpa, log)
	if err != nil {
		return err
	}
	askCfg, err := aliyunutil.GetASKConfigs(mpa, log)
	if err != nil {
		return err
	}

	cli, err := sdk.NewClientWithAccessKey(askCfg.RegionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	clusterID, err := aliyunutil.GetClusterIDByName(cli, vc.Name, askCfg.RegionID)
	if err != nil {
		return err
	}

	err = aliyunutil.SendDeletionRequest(cli, clusterID, askCfg.RegionID)
	if err != nil {
		return err
	}

	// block until the ASK is deleted or timeout after 120 seconds
	deletionTimeout := time.After(100 * time.Second)
OuterLoop:
	for {
		select {
		case <-time.After(2 * time.Second):
			_, state, err := aliyunutil.GetASKStateAndSlbID(cli, clusterID, askCfg.RegionID)
			if err != nil {
				if aliyunutil.IsSDKErr(err) {
					if aliyunutil.GetSDKErrCode(err) == aliyunutil.ClusterNotFound {
						log.Info("corresponding ASK cluster is not found", "vc-name", vc.Name)
						break OuterLoop
					}
				}
				return err
			}
			if state == "deleting" {
				// once the ASK cluster enter the 'deleting' state, the cloud
				// provider will delete the cluster
				log.Info("ASK cluster is being deleted")
				break OuterLoop
			}
		case <-deletionTimeout:
			return fmt.Errorf("Delete ASK(%s) timeout", vc.Name)
		}
	}

	return nil
}

func (mpa *MasterProvisionerAliyun) GetMasterProvisioner() string {
	return "aliyun"
}
