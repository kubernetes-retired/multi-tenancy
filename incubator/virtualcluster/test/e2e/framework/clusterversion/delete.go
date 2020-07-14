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

package clusterversion

import (
	"fmt"

	apierrs "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/test/e2e/framework"
)

func DeleteCV(client vcclient.Interface, cv *v1alpha1.ClusterVersion) error {
	if cv == nil {
		return nil
	}
	return DeleteCVByName(client, cv.GetName())
}

func DeleteCVByName(client vcclient.Interface, name string) error {
	framework.Logf("Deleting cv %q", name)
	err := client.TenancyV1alpha1().ClusterVersions().Delete(name, nil)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("clusterVersion delete API error: %v", err)
	}
	return nil
}
