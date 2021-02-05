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

package main

import (
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/configmap"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/endpoints"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/event"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/namespace"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/node"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolume"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolumeclaim"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/pod"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/secret"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/service"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/serviceaccount"
	_ "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/storageclass"
)
