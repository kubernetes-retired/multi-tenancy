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

package native

import (
	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode"
)

type provider struct {
	vnAgentPort int32
}

var _ vnode.VirtualNodeProvider = &provider{}

func NewNativeVirtualNodeProvider(vnAgentPort int32) vnode.VirtualNodeProvider {
	return &provider{vnAgentPort: vnAgentPort}
}

func (p *provider) GetNodeDaemonEndpoints(node *v1.Node) (v1.NodeDaemonEndpoints, error) {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.vnAgentPort,
		},
	}, nil
}

func (p *provider) GetNodeAddress(node *v1.Node) ([]v1.NodeAddress, error) {
	var addresses []v1.NodeAddress
	for _, a := range node.Status.Addresses {
		// notes: drop host name address because tenant apiserver using cluster dns.
		// It could not find the node by hostname through this dns.
		if a.Type != v1.NodeHostName {
			addresses = append(addresses, a)
		}
	}

	return addresses, nil
}
