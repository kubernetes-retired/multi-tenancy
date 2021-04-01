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

package vnode

import (
	"context"
	"encoding/json"
	"fmt"

	pkgerr "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/native"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/provider"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/service"
	utilconstants "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
)

func GetNodeProvider(config *config.SyncerConfiguration, client clientset.Interface) provider.VirtualNodeProvider {
	if featuregate.DefaultFeatureGate.Enabled(featuregate.VNodeProviderService) {
		return service.NewServiceVirtualNodeProvider(config.VNAgentPort, config.VNAgentNamespacedName, client)
	}
	return native.NewNativeVirtualNodeProvider(config.VNAgentPort)
}

func NewVirtualNode(provider provider.VirtualNodeProvider, node *v1.Node) (vnode *v1.Node, err error) {
	now := metav1.Now()
	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: v1.NodeSpec{
			Unschedulable: true,
			Taints: []v1.Taint{
				{
					Key:       "node.kubernetes.io/unschedulable",
					Effect:    v1.TaintEffectNoSchedule,
					TimeAdded: &now,
				},
			},
		},
	}

	labels := map[string]string{
		constants.LabelVirtualNode: "true",
	}

	if featuregate.DefaultFeatureGate.Enabled(featuregate.SuperClusterPooling) {
		labels[constants.LabelSuperClusterID] = utilconstants.SuperClusterID
	}

	for k, v := range node.GetLabels() {
		if _, isWellKnown := wellKnownNodeLabelsMap[k]; isWellKnown {
			labels[k] = v
		}
	}
	n.SetLabels(labels)

	// fill in status
	n.Status.Conditions = nodeConditions()
	n.Status.NodeInfo.OperatingSystem = "Linux"
	de, err := provider.GetNodeDaemonEndpoints(node)
	if err != nil {
		return nil, pkgerr.Wrapf(err, "get node daemon endpoints from provider")
	}
	n.Status.DaemonEndpoints = de

	na, err := provider.GetNodeAddress(node)
	if err != nil {
		return nil, pkgerr.Wrapf(err, "get node address from provider")
	}

	n.Status.Addresses = na
	n.Status.NodeInfo = node.Status.NodeInfo
	n.Status.Capacity = node.Status.Capacity
	n.Status.Allocatable = node.Status.Allocatable

	return n, nil
}

var wellKnownNodeLabelsMap = map[string]struct{}{
	v1.LabelOSStable:   {},
	v1.LabelArchStable: {},
	v1.LabelHostname:   {},
}

func nodeConditions() []v1.NodeCondition {
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionTrue,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is ready.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}
}

func UpdateNodeStatus(client v1core.NodeInterface, node, newNode *v1.Node) error {
	_, _, err := patchNodeStatus(client, types.NodeName(node.Name), node, newNode)
	return err
}

// patchNodeStatus patches node status.
// Copied from github.com/kubernetes/kubernetes/pkg/util/node
func patchNodeStatus(nodes v1core.NodeInterface, nodeName types.NodeName, oldNode *v1.Node, newNode *v1.Node) (*v1.Node, []byte, error) {
	patchBytes, err := preparePatchBytesforNodeStatus(nodeName, oldNode, newNode)
	if err != nil {
		return nil, nil, err
	}

	updatedNode, err := nodes.Patch(context.TODO(), string(nodeName), types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}, "status")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to patch status %q for node %q: %v", patchBytes, nodeName, err)
	}
	return updatedNode, patchBytes, nil
}

func preparePatchBytesforNodeStatus(nodeName types.NodeName, oldNode *v1.Node, newNode *v1.Node) ([]byte, error) {
	oldData, err := json.Marshal(oldNode)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal oldData for node %q: %v", nodeName, err)
	}

	// NodeStatus.Addresses is incorrectly annotated as patchStrategy=merge, which
	// will cause strategicpatch.CreateTwoWayMergePatch to create an incorrect patch
	// if it changed.
	manuallyPatchAddresses := (len(oldNode.Status.Addresses) > 0) && !equality.Semantic.DeepEqual(oldNode.Status.Addresses, newNode.Status.Addresses)

	var newAddresses []v1.NodeAddress
	if manuallyPatchAddresses {
		newAddresses = newNode.Status.Addresses
		newNode.Status.Addresses = oldNode.Status.Addresses
	}
	newData, err := json.Marshal(newNode)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal newData for node %q: %v", nodeName, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Node{})
	if err != nil {
		return nil, fmt.Errorf("failed to CreateTwoWayMergePatch for node %q: %v", nodeName, err)
	}

	if manuallyPatchAddresses {
		patchBytes, err = fixupPatchForNodeStatusAddresses(patchBytes, newAddresses)
		if err != nil {
			return nil, fmt.Errorf("failed to fix up NodeAddresses in patch for node %q: %v", nodeName, err)
		}
	}
	return patchBytes, nil
}

// fixupPatchForNodeStatusAddresses adds a replace-strategy patch for Status.Addresses to
// the existing patch
func fixupPatchForNodeStatusAddresses(patchBytes []byte, addresses []v1.NodeAddress) ([]byte, error) {
	// Given patchBytes='{"status": {"conditions": [ ... ], "phase": ...}}' and
	// addresses=[{"type": "InternalIP", "address": "10.0.0.1"}], we need to generate:
	//
	//   {
	//     "status": {
	//       "conditions": [ ... ],
	//       "phase": ...,
	//       "addresses": [
	//         {
	//           "type": "InternalIP",
	//           "address": "10.0.0.1"
	//         },
	//         {
	//           "$patch": "replace"
	//         }
	//       ]
	//     }
	//   }

	var patchMap map[string]interface{}
	if err := json.Unmarshal(patchBytes, &patchMap); err != nil {
		return nil, err
	}

	addrBytes, err := json.Marshal(addresses)
	if err != nil {
		return nil, err
	}
	var addrArray []interface{}
	if err := json.Unmarshal(addrBytes, &addrArray); err != nil {
		return nil, err
	}
	addrArray = append(addrArray, map[string]interface{}{"$patch": "replace"})

	status := patchMap["status"]
	if status == nil {
		status = map[string]interface{}{}
		patchMap["status"] = status
	}
	statusMap, ok := status.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected data in patch")
	}
	statusMap["addresses"] = addrArray

	return json.Marshal(patchMap)
}
