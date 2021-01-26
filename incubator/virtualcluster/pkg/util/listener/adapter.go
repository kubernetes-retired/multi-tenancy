/*
Copyright 2021 The Kubernetes Authors.

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

package listener

import (
	"k8s.io/klog"

	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type MCControllerListener struct {
	c *mc.MultiClusterController
}

var _ ClusterChangeListener = &MCControllerListener{}

func NewMCControllerListener(c *mc.MultiClusterController) ClusterChangeListener {
	return &MCControllerListener{c: c}
}

func (m MCControllerListener) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("%s watch cluster %s for %s resource", m.c.GetControllerName(), cluster.GetClusterName(), m.c.GetObjectKind())
	err := m.c.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s %s event: %v", cluster.GetClusterName(), m.c.GetObjectKind(), err)
	}
}

func (m MCControllerListener) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("%s stop watching cluster %s for %s resource", m.c.GetControllerName(), cluster.GetClusterName(), m.c.GetObjectKind())
	m.c.TeardownClusterResource(cluster)
}
