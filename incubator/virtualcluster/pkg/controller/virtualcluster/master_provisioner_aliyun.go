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
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/secret"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	VcManagerNs = "vc-manager"

	// consts used to get aliyun accesskey ID/Secret from secret
	AliyunAkSrt        = "aliyun-accesskey"
	AliyunAKIDName     = "accessKeyID"
	AliyunAKSecretName = "accessKeySecret"

	// consts used to get ask configuration from ConfigMap
	AliyunASKConfigMap     = "aliyun-ask-config"
	AliyunASKCfgMpRegionID = "askRegionID"
	AliyunASKCfgMpZoneID   = "askZoneID"

	AnnotationClusterIDKey = "clusterID"
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

// sendCreationRequest sends ASK creation request to aliyun
func sendCreationRequest(cli *sdk.Client, clusterName, accessKeyID, accessKeySecret, regionID, zoneID string) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "https"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = "/clusters"
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
	body := fmt.Sprintf(`{
"cluster_type": "Ask",
"name": "%s", 
"region_id": "%s",
"zoneid": "%s",
"nat_gateway": true,
"private_zone": true
}`, clusterName, regionID, zoneID)
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

// getASKState gets the latest state of the ASK with the given clusterID
func getASKState(cli *sdk.Client, clusterID, regionID string) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "GET"
	request.Scheme = "https"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = fmt.Sprintf("/clusters/%s", clusterID)
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
	response, err := cli.ProcessCommonRequest(request)
	if err != nil {
		return "", err
	}

	var clsInfo map[string]interface{}
	if err := json.Unmarshal(response.GetHttpContentBytes(), &clsInfo); err != nil {
		return "", err
	}
	clsIDInf, exist := clsInfo["cluster_id"]
	if !exist {
		return "", errors.New("cluster info entry doesn't contain 'cluster_id' field")
	}
	clsID, ok := clsIDInf.(string)
	if !ok {
		return "", errors.New("fail to assert cluster id")
	}
	// find desired cluster
	if clsID != clusterID {
		return "", fmt.Errorf("cluster id does not match: got %s want %s", clsID, clusterID)
	}
	clsStateInf, exist := clsInfo["state"]
	if !exist {
		return "", fmt.Errorf("fail to get 'state' of cluster(%s)", clusterID)
	}
	clsState, ok := clsStateInf.(string)
	if !ok {
		return "", fmt.Errorf("fail to assert cluster.state to string")
	}

	return clsState, nil
}

// getASKPrivateKubeConfig retrieves the kubeconfig of the ASK with the given clusterID.
// NOTE if internalNetwork is set, the return kubeconfig will use private IP address as the ASK server's address.
func getASKKubeConfig(cli *sdk.Client, clusterID, regionID string) (string, error) {
	request := requests.NewCommonRequest()
	request.Method = "GET"
	request.Scheme = "https"
	request.Domain = "cs.aliyuncs.com"
	request.Version = "2015-12-15"
	request.PathPattern = fmt.Sprintf("/k8s/%s/user_config", clusterID)
	request.Headers["Content-Type"] = "application/json"
	request.QueryParams["RegionId"] = regionID
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
	request.Scheme = "https"
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
	akSrt := &corev1.Secret{}
	if getErr := mpa.Get(context.TODO(), types.NamespacedName{
		Namespace: VcManagerNs,
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
func (mpa *MasterProvisionerAliyun) getASKConfigs() (regionID, zoneID string, err error) {
	askCfgMp := &corev1.ConfigMap{}
	if getErr := mpa.Get(context.TODO(), types.NamespacedName{
		Namespace: VcManagerNs,
		Name:      AliyunASKConfigMap,
	}, askCfgMp); getErr != nil {
		err = getErr
	}

	regionID, exist := askCfgMp.Data[AliyunASKCfgMpRegionID]
	if !exist {
		err = fmt.Errorf("%s not exist", AliyunASKCfgMpRegionID)
		return
	}

	zoneID, exist = askCfgMp.Data[AliyunASKCfgMpZoneID]
	if !exist {
		err = fmt.Errorf("%s not exist", AliyunASKCfgMpZoneID)
		return
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

	regionID, zoneID, err := mpa.getASKConfigs()
	if err != nil {
		return err
	}

	// 2. send ASK creation request
	// NOTE http requests of a creation action will be sent by a same client
	cli, err := sdk.NewClientWithAccessKey(regionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	clsID, err := sendCreationRequest(cli, vc.Name, aliyunAKID, aliyunAKSrt, regionID, zoneID)
	if err != nil {
		return err
	}

	log.Info("ASK is creating", "ASK-ID", clsID)

	// 3. block until the newly created ASK is up and running
	creationTimeout := time.After(120 * time.Second)

OuterLoop:
	for {
		select {
		case <-time.After(10 * time.Second):
			clsState, err := getASKState(cli, clsID, regionID)
			if err != nil {
				return err
			}
			if clsState == "running" {
				// ASK is up and running, stop polling
				log.Info("ASK is up and running", "ASK-ID", clsID)
				break OuterLoop
			}
		case <-creationTimeout:
			return fmt.Errorf("creating cluster(%s) timeout", clsID)
		}
	}

	// 4. embed clusterID into the Virtualcluster CR
	vc.ObjectMeta.Annotations["clusterID"] = clsID
	if err = mpa.Update(context.TODO(), vc); err != nil {
		return err
	}

	// 5. create the root namesapce of the Virtualcluster
	vcNs := conversion.ToClusterKey(vc)
	err = mpa.Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: conversion.ToClusterKey(vc),
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	log.Info("virtualcluster ns is created", "ns", conversion.ToClusterKey(vc))

	// 6. get kubeconfig of the newly created ASK
	kbCfg, err := getASKKubeConfig(cli, clsID, regionID)
	if err != nil {
		return err
	}
	log.Info("got kubeconfig of cluster", "cluster", clsID)

	// 7. serialize kubeconfig to secret and store it in the
	// corresponding namespace (i.e.)
	adminSrt := secret.KubeconfigToSecret(secret.AdminSecretName,
		vcNs, kbCfg)
	err = mpa.Create(context.TODO(), adminSrt)
	if err != nil {
		return err
	}

	log.Info("admin kubeconfig is created for virtualcluster", "vc", vc.Name)
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
	regionID, _, err := mpa.getASKConfigs()
	if err != nil {
		return err
	}

	cli, err := sdk.NewClientWithAccessKey(regionID, aliyunAKID, aliyunAKSrt)
	if err != nil {
		return err
	}

	clusterID, exist := vc.ObjectMeta.Annotations[AnnotationClusterIDKey]
	if !exist {
		return fmt.Errorf("Virtualcluster(%s) doesn't have %s annotation", vc.Name, AnnotationClusterIDKey)
	}

	err = sendDeletionRequest(cli, clusterID, regionID)
	if err != nil {
		return err
	}

	// block until the ASK is deleted or timeout after 120 seconds
	deletionTimeout := time.After(100 * time.Second)
OuterLoop:
	for {
		select {
		case <-time.After(2 * time.Second):
			state, err := getASKState(cli, clusterID, regionID)
			if err != nil {
				return err
			}
			if state == "deleting" {
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
