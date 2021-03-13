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

package service

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func newNode() *v1.Node {
	return &v1.Node{
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				v1.NodeAddress{
					Type:    v1.NodeInternalIP,
					Address: "192.168.0.2",
				},
			},
		},
	}
}

func newClient() clientset.Interface {
	return fake.NewSimpleClientset(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vn-agent",
			Namespace: "vc-manager",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "192.168.0.5",
		},
	})
}

func Test_provider_GetNodeAddress(t *testing.T) {
	type fields struct {
		vnAgentPort          int32
		vnAgentNamespaceName string
		client               clientset.Interface
	}
	type args struct {
		node *v1.Node
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []v1.NodeAddress
		wantErr bool
	}{
		{
			name: "TestWithNoService",
			fields: fields{
				vnAgentNamespaceName: "default/vn-agent",
				client:               newClient(),
			},
			args:    args{newNode()},
			want:    []v1.NodeAddress{},
			wantErr: true,
		},
		{
			name: "TestWithService",
			fields: fields{
				vnAgentNamespaceName: "vc-manager/vn-agent",
				client:               newClient(),
			},
			args: args{newNode()},
			want: []v1.NodeAddress{
				v1.NodeAddress{
					Type:    v1.NodeInternalIP,
					Address: "192.168.0.5",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &provider{
				vnAgentPort:          tt.fields.vnAgentPort,
				vnAgentNamespaceName: tt.fields.vnAgentNamespaceName,
				client:               tt.fields.client,
			}
			got, err := p.GetNodeAddress(tt.args.node)
			if (err != nil) != tt.wantErr {
				t.Errorf("provider.GetNodeAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(tt.want) != 0 && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("provider.GetNodeAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}
