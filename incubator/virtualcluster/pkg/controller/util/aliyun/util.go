/*
Copyright 2020 The Kubernetes Authors.

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

package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	strutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/strings"
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

	// AnnotationClusterID is the cluster id of the remote virtualcluster master on the cloud
	AnnotationClusterID = "tenancy.x-k8s.io/ask.clusterID"
	// AnnotationSlbID is the loadbalancer id of the remote virtualcluster master on the cloud
	AnnotationSlbID = "tenancy.x-k8s.io/ask.slbID"
	// AnnotationKubeconfig is the admin-kubeconfig to access the remote virtualcluster master on the cloud
	AnnotationKubeconfig = "tenancy.x-k8s.io/admin-kubeconfig"
)

type ASKConfig struct {
	VPCID        string
	VSwitchID    string
	RegionID     string
	ZoneID       string
	PrivateKbCfg string
}

const (
	// full list of potential API errors can be found at
	// https://error-center.alibabacloud.com/status/product/Cos?spm=a2c69.11428812.home.7.2247bb9adTOFxm
	OprationNotSupported    = "ErrorCheckAcl"
	ClusterNotFound         = "ErrorClusterNotFound"
	ClusterNameAlreadyExist = "ClusterNameAlreadyExist"
	QueryClusterError       = "ErrorQueryCluster"
)

// GetASKPrivateKubeConfig retrieves the kubeconfig of the ASK with the given clusterID.
func GetASKKubeConfig(cli *sdk.Client, clusterID, regionID, privateKbCfg string) (string, error) {
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

// GetASKStateAndSlbID gets the slb ID (external_loadbalncer id) and the latest
// state of the ASK with the given clusterID
func GetASKStateAndSlbID(cli *sdk.Client, clusterID, regionID string) (slbId, state string, err error) {
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

// GetClusterIDByName returns the clusterID of the cluster with clusterName
func GetClusterIDByName(cli *sdk.Client, clusterName, regionID string) (string, error) {
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

func GetSDKErrCode(err error) string {
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

func IsSDKErr(err error) bool {
	return strings.HasPrefix(err.Error(), "SDK.ServerError")
}

// SendCreationRequest sends ASK creation request to Aliyun. If there exists an ASK
// with the same clusterName, retrieve and return the clusterID of the ASK instead of
// creating a new one
func SendCreationRequest(cli *sdk.Client, clusterName string, askCfg ASKConfig) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "http"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = "/clusters"
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = askCfg.RegionID

	// set vpc, if VPCID is specified
	var body string
	if askCfg.VPCID != "" {
		body = fmt.Sprintf(`{
"cluster_type": "Ask",
"name": "%s", 
"region_id": "%s",
"zoneid": "%s", 
"vpc_id": "%s",
"vswitch_id": "%s",
"nat_gateway": false,
"private_zone": true
}`, clusterName, askCfg.RegionID, askCfg.ZoneID, askCfg.VPCID, askCfg.VSwitchID)
	} else {
		body = fmt.Sprintf(`{
"cluster_type": "Ask",
"name": "%s", 
"region_id": "%s",
"zoneid": "%s", 
"nat_gateway": true,
"private_zone": true
}`, clusterName, askCfg.RegionID, askCfg.ZoneID)
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

// SendDeletionRequest sends a request for deleting the ASK with the given clusterID
func SendDeletionRequest(cli *sdk.Client, clusterID, regionID string) error {
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

// GetAliyunAKPair gets the current aliyun AccessKeyID/AccessKeySecret from secret
// NOTE AccessKeyID/AccessKeySecret may be changed if user update the secret `aliyun-accesskey`
func GetAliyunAKPair(cli client.Client, log logr.Logger) (keyID string, keySecret string, err error) {
	var vcManagerNs string
	vcManagerNs, getNsErr := kubeutil.GetPodNsFromInside()
	if getNsErr != nil {
		log.Info("can't find NS from inside the pod", "err", err)
		vcManagerNs = DefaultVcManagerNs
	}
	akSrt := &v1.Secret{}
	if getErr := cli.Get(context.TODO(), types.NamespacedName{
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

// GetASKConfigs gets the ASK configuration information from ConfigMap
func GetASKConfigs(cli client.Client, log logr.Logger) (cfg ASKConfig, err error) {
	var vcManagerNs string
	vcManagerNs, getNsErr := kubeutil.GetPodNsFromInside()
	if getNsErr != nil {
		log.Info("can't find NS from inside the pod", "err", err)
		vcManagerNs = DefaultVcManagerNs
	}

	ASKCfgMp := &v1.ConfigMap{}
	if getErr := cli.Get(context.TODO(), types.NamespacedName{
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
	cfg.RegionID = regionID

	zoneID, ziExist := ASKCfgMp.Data[AliyunASKCfgMpZoneID]
	if !ziExist {
		err = fmt.Errorf("%s not exist", AliyunASKCfgMpZoneID)
		return
	}
	cfg.ZoneID = zoneID

	privateKbCfg, pkcExist := ASKCfgMp.Data[AliyunASKCfgMpPrivateCfg]
	// cfg.privateKbCfg can only be set as "true" or "false"
	if pkcExist {
		if privateKbCfg == "true" || privateKbCfg == "false" {
			cfg.PrivateKbCfg = privateKbCfg
		} else {
			err = fmt.Errorf("%s.data.%s can only be set as 'true' or 'false'",
				AliyunASKConfigMap, AliyunASKCfgMpPrivateCfg)
			return
		}
	} else {
		cfg.PrivateKbCfg = "false"
	}

	vpcID, viExist := ASKCfgMp.Data[AliyunASKCfgMpVPCID]
	vsID, vsiExist := ASKCfgMp.Data[AliyunASKCfgMpVSID]
	if viExist != vsiExist {
		err = errors.New("vswitchID and vpcID need to be used together")
	}

	if viExist && vsiExist {
		cfg.VPCID = vpcID
		cfg.VSwitchID = vsID
	}

	return
}
