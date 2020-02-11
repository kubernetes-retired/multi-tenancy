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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
	kubeutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	strutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/util/strings"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	DefaultVcManagerNs = "vc-manager"

	// consts used to get aliyun accesskey ID/Secret from secret
	AliyunAkSrt        = "aliyun-accesskey"
	AliyunAKIDName     = "accessKeyID"
	AliyunAKSecretName = "accessKeySecret"

	// consts used to get ask configuration from ConfigMap
	AliyunASKConfigMap       = "aliyun-ask-config"
	AliyunASKCfgMpRegionID   = "askRegionID"
	AliyunASKCfgMpZoneID     = "askZoneID"
	AliyunASKCfgMpVPCID      = "askVpcID"
	AliyunASKCfgMpVSID       = "askVswitchID"
	AliyunASKCfgMpPrivateCfg = "askPrivateKbCfg"

	AnnotationClusterIDKey = "tenancy.x-k8s.io/ask.clusterID"
	AnnotationSlbIDKey     = "tenancy.x-k8s.io/ask.slbID"
)

type ASKConfig struct {
	vpcID        string
	vswitchID    string
	regionID     string
	zoneID       string
	privateKbCfg string
}

const (
	// full list of potential API errors can be found at
	// https://error-center.alibabacloud.com/status/product/Cos?spm=a2c69.11428812.home.7.2247bb9adTOFxm
	OprationNotSupported    = "ErrorCheckAcl"
	ClusterNotFound         = "ErrorClusterNotFound"
	ClusterNameAlreadyExist = "ClusterNameAlreadyExist"
	QueryClusterError       = "ErrorQueryCluster"
)

type MasterProvisionerAliyun struct {
	client.Client
	scheme *runtime.Scheme
}

func NewMasterProvisionerAliyun(mgr manager.Manager) *MasterProvisionerAliyun {
	return &MasterProvisionerAliyun{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// getClusterIDByName returns the clusterID of the cluster with clusterName
func getClusterIDByName(cli *sdk.Client, clusterName, regionID string) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "GET"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = "/clusters"
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
	response, err := cli.ProcessCommonRequest(request)
	if err != nil {
		return "", err
	}

	var clsInfoLst []map[string]interface{}
	if err := json.Unmarshal(response.GetHttpContentBytes(), &clsInfoLst); err != nil {
		return "", err
	}
	for _, clsInfo := range clsInfoLst {
		clsNameInf, exist := clsInfo["name"]
		if !exist {
			return "", errors.New("clusterInfo doesn't contain 'name' field")
		}
		clsName, ok := clsNameInf.(string)
		if !ok {
			return "", errors.New("fail to assert 'name' to string")
		}
		if clsName == clusterName {
			clsIDInf, exist := clsInfo["cluster_id"]
			if !exist {
				return "", errors.New("clusterInfo doesn't contain 'cluster_id' field")
			}
			clsID, ok := clsIDInf.(string)
			if !ok {
				return "", errors.New("fail to assert 'cluster_id' to string")
			}
			return clsID, nil
		}
	}
	return "", fmt.Errorf("can't find cluster information for cluster(%s)", clusterName)
}

// sendCreationRequest sends ASK creation request to Aliyun. If there exists an ASK
// with the same clusterName, retrieve and return the clusterID of the ASK instead of
// creating a new one
func sendCreationRequest(cli *sdk.Client, clusterName string, askCfg ASKConfig) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = "/clusters"
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = askCfg.regionID

	// set vpc, if vpcID is specified
	var body string
	if askCfg.vpcID != "" {
		body = fmt.Sprintf(`{
"cluster_type": "Ask",
"name": "%s", 
"region_id": "%s",
"zoneid": "%s", 
"vpc_id": "%s",
"vswitch_id": "%s",
"nat_gateway": false,
"private_zone": true
}`, clusterName, askCfg.regionID, askCfg.zoneID, askCfg.vpcID, askCfg.vswitchID)
	} else {
		body = fmt.Sprintf(`{
"cluster_type": "Ask",
"name": "%s", 
"region_id": "%s",
"zoneid": "%s", 
"nat_gateway": true,
"private_zone": true
}`, clusterName, askCfg.regionID, askCfg.zoneID)
	}

	request.Content = []byte(body)
	response, err := cli.ProcessCommonRequest(request)
	if err != nil {

		return "", err
	}

	// cluster information of the newly created ASK in json format
	clsInfo := make(map[string]string)
	if err := json.Unmarshal(response.GetHttpContentBytes(), &clsInfo); err != nil {
		return "", err
	}
	clusterID, exist := clsInfo["cluster_id"]
	if !exist {
		return "", errors.New("can't find 'cluster_id' in response body")
	}
	return clusterID, nil
}

func isSDKErr(err error) bool {
	return strings.HasPrefix(err.Error(), "SDK.ServerError")
}

func getSDKErrCode(err error) string {
	// an SDK error looks like:
	//
	// SDK.ServerError
	// ErrorCode:
	// Recommend:
	// RequestId:
	// Message: {"code":"ClusterNameAlreadyExist","message":"cluster name {XXX} already exist in your clusters","requestId":"C2D0F836-DD3D-4749-97AB-10AE8371BABE","status":400}
	errMsg := strings.Split(err.Error(), "\n")[4]
	errCodeWithQuote := strutil.SplitFields(errMsg, ':', ',')[2]
	// remove surrounding quotes
	return errCodeWithQuote[1 : len(errCodeWithQuote)-1]
}

// getASKStateAndSlbID gets the slb ID (external_loadbalncer id) and the latest
// state of the ASK with the given clusterID
func getASKStateAndSlbID(cli *sdk.Client, clusterID, regionID string) (slbId, state string, err error) {
	request := requests.NewCommonRequest()
	request.Method = "GET"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = fmt.Sprintf("/clusters/%s", clusterID)
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID

	response, err := cli.ProcessCommonRequest(request)
	if err != nil {
		return
	}

	var clsInfo map[string]interface{}
	if err = json.Unmarshal(response.GetHttpContentBytes(), &clsInfo); err != nil {
		return
	}
	clsIDInf, exist := clsInfo["cluster_id"]
	if !exist {
		err = errors.New("cluster info entry doesn't contain 'cluster_id' field")
		return
	}
	clsID, ok := clsIDInf.(string)
	if !ok {
		err = errors.New("fail to assert cluster id")
		return
	}
	// find desired cluster
	if clsID != clusterID {
		err = fmt.Errorf("cluster id does not match: got %s want %s", clsID, clusterID)
		return
	}
	clsStateInf, exist := clsInfo["state"]
	if !exist {
		err = fmt.Errorf("fail to get 'state' of cluster(%s)", clusterID)
		return
	}
	clsSlbIdInf, exist := clsInfo["external_loadbalancer_id"]
	if !exist {
		err = fmt.Errorf("fail to get 'external_loadbalancer_id' of cluster(%s)", clusterID)
		return
	}

	slbId, ok = clsSlbIdInf.(string)
	if !ok {
		err = fmt.Errorf("fail to assert cluster.external_loadbalancer_idstring")
		return
	}

	state, ok = clsStateInf.(string)
	if !ok {
		err = fmt.Errorf("fail to assert cluster.state to string")
		return
	}

	return
}

// getASKPrivateKubeConfig retrieves the kubeconfig of the ASK with the given clusterID.
func getASKKubeConfig(cli *sdk.Client, clusterID, regionID, privateKbCfg string) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "GET"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = fmt.Sprintf("/k8s/%s/user_config", clusterID)
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
	if privateKbCfg != "" {
		// if specified, get kubeconfig that uses private IP
		request.QueryParams["PrivateIpAddress"] = privateKbCfg
	}
	response, err := cli.ProcessCommonRequest(request)
	if err != nil {
		return "", err
	}
	kbCfgJson := make(map[string]string)
	if err := json.Unmarshal(response.GetHttpContentBytes(), &kbCfgJson); err != nil {
		return "", err
	}

	kbCfg, exist := kbCfgJson["config"]
	if !exist {
		return "", fmt.Errorf("kubeconfig of cluster(%s) is not found", clusterID)
	}
	return kbCfg, nil
}

// sendDeletionRequest sends a request for deleting the ASK with the given clusterID
func sendDeletionRequest(cli *sdk.Client, clusterID, regionID string) error {
	request := requests.NewCommonRequest()
	request.Method = "DELETE"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = fmt.Sprintf("/clusters/%s", clusterID)
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
	_, err := cli.ProcessCommonRequest(request)
	if err != nil {
		return err
	}
	return nil
}

// getAliyunAKPair gets the current aliyun AccessKeyID/AccessKeySecret from secret
// NOTE AccessKeyID/AccessKeySecret may be changed if user update the secret `aliyun-accesskey`
func (mpa *MasterProvisionerAliyun) getAliyunAKPair() (keyID string, keySecret string, err error) {
	var vcManagerNs string
	vcManagerNs, getNsErr := kubeutil.GetPodNsFromInside()
	if getNsErr != nil {
		log.Info("can't find NS from inside the pod", "err", err)
		vcManagerNs = DefaultVcManagerNs
	}
	akSrt := &corev1.Secret{}
	if getErr := mpa.Get(context.TODO(), types.NamespacedName{
		Namespace: vcManagerNs,
		Name:      AliyunAkSrt,
	}, akSrt); getErr != nil {
		err = getErr
	}

	keyIDByt, exist := akSrt.Data[AliyunAKIDName]
	if !exist {
		err = errors.New("aliyun accessKeyID doesn't exist")
	}
	keyID = string(keyIDByt)

	keySrtByt, exist := akSrt.Data[AliyunAKSecretName]
	if !exist {
		err = errors.New("aliyun accessKeySecret doesn't exist")
	}
	keySecret = string(keySrtByt)
	return
}

// getASKConfigs gets the ASK configuration information from ConfigMap
func (mpa *MasterProvisionerAliyun) getASKConfigs() (cfg ASKConfig, err error) {
	var vcManagerNs string
	vcManagerNs, getNsErr := kubeutil.GetPodNsFromInside()
	if getNsErr != nil {
		log.Info("can't find NS from inside the pod", "err", err)
		vcManagerNs = DefaultVcManagerNs
	}

	ASKCfgMp := &corev1.ConfigMap{}
	if getErr := mpa.Get(context.TODO(), types.NamespacedName{
		Namespace: vcManagerNs,
		Name:      AliyunASKConfigMap,
	}, ASKCfgMp); getErr != nil {
		err = getErr
	}

	regionID, riExist := ASKCfgMp.Data[AliyunASKCfgMpRegionID]
	if !riExist {
		err = fmt.Errorf("%s not exist", AliyunASKCfgMpRegionID)
		return
	}
	cfg.regionID = regionID

	zoneID, ziExist := ASKCfgMp.Data[AliyunASKCfgMpZoneID]
	if !ziExist {
		err = fmt.Errorf("%s not exist", AliyunASKCfgMpZoneID)
		return
	}
	cfg.zoneID = zoneID

	privateKbCfg, pkcExist := ASKCfgMp.Data[AliyunASKCfgMpPrivateCfg]
	// cfg.privateKbCfg can only be set as "true" or "false"
	if pkcExist {
		if privateKbCfg == "true" || privateKbCfg == "false" {
			cfg.privateKbCfg = privateKbCfg
		} else {
			err = fmt.Errorf("%s.data.%s can only be set as 'true' or 'false'",
				AliyunASKConfigMap, AliyunASKCfgMpPrivateCfg)
			return
		}
	} else {
		cfg.privateKbCfg = "false"
	}

	vpcID, viExist := ASKCfgMp.Data[AliyunASKCfgMpVPCID]
	vsID, vsiExist := ASKCfgMp.Data[AliyunASKCfgMpVSID]
	if viExist != vsiExist {
		err = errors.New("vswitchID and vpcID need to be used together")
	}

	if viExist && vsiExist {
		cfg.vpcID = vpcID
		cfg.vswitchID = vsID
	}

	return
}

// CreateVirtualCluster creates a new ASK on aliyun for given Virtualcluster
func (mpa *MasterProvisionerAliyun) CreateVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	log.Info("setting up control plane for the Virtualcluster", "Virtualcluster", vc.Name)
	// 1. load aliyun accessKeyID/accessKeySecret from secret
	aliyunAKID, aliyunAKSrt, err := mpa.getAliyunAKPair()
	if err != nil {
		return err
	}

	askCfg, err := mpa.getASKConfigs()
	if err != nil {
		return err
	}

	// 2. send ASK creation request
	// NOTE http requests of a creation action will be sent by a same client
	cli, err := sdk.NewClientWithAccessKey(askCfg.regionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	var (
		clsID    string
		clsState string
		clsSlbId string
	)
	creationTimeout := time.After(100 * time.Second)
	clsID, err = sendCreationRequest(cli, vc.Name, askCfg)
	if err != nil {
		if !isSDKErr(err) {
			return err
		}
		// check SDK error code
		if getSDKErrCode(err) == ClusterNameAlreadyExist {
			// clusterName already exists, query Aliyun to get the clusterID
			// corresponding to the clusterName
			var getClsIDErr error
			clsID, getClsIDErr = getClusterIDByName(cli, vc.Name, askCfg.regionID)
			if getClsIDErr != nil {
				return getClsIDErr
			}
			var getStErr error
			clsSlbId, clsState, getStErr = getASKStateAndSlbID(cli, clsID, askCfg.regionID)
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
			clsSlbId, clsState, getStErr = getASKStateAndSlbID(cli, clsID, askCfg.regionID)
			if getStErr != nil {
				return getStErr
			}

		case <-creationTimeout:
			return fmt.Errorf("creating cluster(%s) timeout", clsID)
		}
	}

	// 4. create the root namesapce of the Virtualcluster
	vcNs := conversion.ToClusterKey(vc)
	// remove root ns if exist.
	// NOTE rootNS may exist for debugging purposes if creation fail
	err = kubeutil.RemoveNS(mpa, vcNs)
	if err != nil {
		return err
	}
	err = kubeutil.CreateNS(mpa, vcNs)
	if err != nil {
		return err
	}
	log.Info("virtualcluster ns is created", "ns", vcNs)

	// 5. get kubeconfig of the newly created ASK
	kbCfg, err := getASKKubeConfig(cli, clsID, askCfg.regionID, askCfg.privateKbCfg)
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

	// 7. get the slb id of the ask, and patch an annotation including the slb
	// id to the vc cr
	err = kubeutil.AnnotateVC(mpa, vc, AnnotationSlbIDKey, clsSlbId, log)
	if err != nil {
		return err
	}
	log.Info("slb id has been added to vc as annotation", "vc", vc.GetName(), "id", clsSlbId)

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

// DeleteVirtualCluster deletes the ASK cluster corresponding to the given Virtualcluster
// NOTE DeleteVirtualCluster only sends the deletion request to Aliyun and do not promise
// the ASK will be deleted
func (mpa *MasterProvisionerAliyun) DeleteVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error {
	log.Info("deleting the ASK of the virtualcluster", "vc-name", vc.Name)
	aliyunAKID, aliyunAKSrt, err := mpa.getAliyunAKPair()
	if err != nil {
		return err
	}
	askCfg, err := mpa.getASKConfigs()
	if err != nil {
		return err
	}

	cli, err := sdk.NewClientWithAccessKey(askCfg.regionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	clusterID, err := getClusterIDByName(cli, vc.Name, askCfg.regionID)
	if err != nil {
		return err
	}

	err = sendDeletionRequest(cli, clusterID, askCfg.regionID)
	if err != nil {
		return err
	}

	// block until the ASK is deleted or timeout after 120 seconds
	deletionTimeout := time.After(100 * time.Second)
OuterLoop:
	for {
		select {
		case <-time.After(2 * time.Second):
			_, state, err := getASKStateAndSlbID(cli, clusterID, askCfg.regionID)
			if err != nil {
				if isSDKErr(err) {
					if getSDKErrCode(err) == ClusterNotFound {
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
	// delete the root ns
	err = kubeutil.RemoveNS(mpa, conversion.ToClusterKey(vc))
	if err != nil {
		return err
	}
	return nil
}

func (mpa *MasterProvisionerAliyun) GetMasterProvisioner() string {
	return "aliyun"
}
