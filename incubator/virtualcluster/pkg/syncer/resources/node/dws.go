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

package node

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterNodeController.Start(stopCh)
}

// The reconcile logic for tenant master node informer, the main purpose is to maintain
// the nodeNameToCluster mapping
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile node %s for cluster %s", request.Name, request.ClusterName)
	vExists := true
	vNodeObj, err := c.multiClusterNodeController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists {
		vNode := vNodeObj.(*v1.Node)
		if vNode.Labels[constants.LabelVirtualNode] != "true" {
			// We only handle virtual nodes created by syncer
			return reconciler.Result{}, nil
		}
		c.Lock()
		if _, exist := c.nodeNameToCluster[request.Name]; !exist {
			c.nodeNameToCluster[request.Name] = make(map[string]struct{})
		}
		c.nodeNameToCluster[request.Name][request.ClusterName] = struct{}{}
		c.Unlock()
	} else {
		c.Lock()
		if _, exists := c.nodeNameToCluster[request.Name]; exists {
			delete(c.nodeNameToCluster[request.Name], request.ClusterName)
		}
		c.Unlock()

	}
	return reconciler.Result{}, nil
}
