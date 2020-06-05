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
package subcmd

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vcctlutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/cmd/vcctl/util"
	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

// Delete deletes the VirtualCluster vcName
func Delete(yaml string) error {
	kbCfg, err := config.GetConfig()
	if err != nil {
		return err
	}
	mgr, err := manager.New(kbCfg,
		manager.Options{MetricsBindAddress: ":8081"})
	if err != nil {
		return err
	}

	err = tenancyv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	ro, err := vcctlutil.YamlToObj(mgr.GetScheme(), yaml)
	if err != nil {
		return err
	}

	vc, ok := ro.(*tenancyv1alpha1.VirtualCluster)
	if !ok {
		return errors.New("please specify a virtualcluster yaml")
	}

	// delete the virtualcluster object
	err = mgr.GetClient().Delete(context.TODO(), vc)
	if err != nil {
		return err
	}

	return nil
}
