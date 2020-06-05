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

package controller

import (
	"fmt"

	vcmanager "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/vcmanager"
)

type ControllerName int

const (
	VirtualClusterController ControllerName = iota
	ClusterversionController
)

func (cn ControllerName) String() string {
	switch cn {
	case VirtualClusterController:
		return "VirtualClusterController"
	case ClusterversionController:
		return "ClusterversionController"
	default:
		return fmt.Sprintf("%d", cn)
	}
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs = make(map[ControllerName]func(*vcmanager.VirtualClusterManager, string) error)

// AddToManager adds all Controllers to the Manager
func AddToManager(m *vcmanager.VirtualClusterManager, masterProvisioner string) error {
	// add controller based the type of the masterProvisioner
	switch masterProvisioner {
	case "native":
		f, exist := AddToManagerFuncs[VirtualClusterController]
		if !exist {
			return fmt.Errorf("%s not found", VirtualClusterController)
		}
		if err := f(m, masterProvisioner); err != nil {
			return err
		}

		f, exist = AddToManagerFuncs[ClusterversionController]
		if !exist {
			return fmt.Errorf("%s not found", ClusterversionController)
		}
		if err := f(m, masterProvisioner); err != nil {
			return err
		}
	case "aliyun":
		f, exist := AddToManagerFuncs[VirtualClusterController]
		if !exist {
			return fmt.Errorf("%s not found", VirtualClusterController)
		}
		if err := f(m, masterProvisioner); err != nil {
			return err
		}
	}
	return nil
}
