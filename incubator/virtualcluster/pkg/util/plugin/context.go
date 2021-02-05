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

package plugin

import (
	"context"

	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
)

// InitContext is used for plugin initialization
type InitContext struct {
	Context    context.Context
	Config     interface{}
	Client     clientset.Interface
	Informer   informers.SharedInformerFactory
	VCClient   vcclient.Interface
	VCInformer vcinformers.VirtualClusterInformer
}
