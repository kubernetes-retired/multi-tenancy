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

package storageclass

import (
	"fmt"

	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.storageclassSynced) {
		return fmt.Errorf("failed to wait for caches to sync storageclass")
	}
	return c.upwardStorageClassController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	// The key format is clsutername/scName.
	clusterName, scName, _ := cache.SplitMetaNamespaceKey(key)

	op := reconciler.AddEvent
	pStorageClass, err := c.storageclassLister.Get(scName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		op = reconciler.DeleteEvent
	}

	tenantClient, err := c.multiClusterStorageClassController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	vStorageClassObj, err := c.multiClusterStorageClassController.Get(clusterName, "", scName)
	if err != nil {
		if errors.IsNotFound(err) {
			if op == reconciler.AddEvent {
				// Available in super, hence create a new in tenant master
				vStorageClass := conversion.BuildVirtualStorageClass(clusterName, pStorageClass)
				_, err := tenantClient.StorageV1().StorageClasses().Create(vStorageClass)
				if err != nil {
					return err
				}
			}
			return nil
		}
		return err
	}

	if op == reconciler.DeleteEvent {
		opts := &metav1.DeleteOptions{
			PropagationPolicy: &constants.DefaultDeletionPolicy,
		}
		err := tenantClient.StorageV1().StorageClasses().Delete(scName, opts)
		if err != nil {
			return err
		}
	} else {
		updatedStorageClass := conversion.Equality(nil).CheckStorageClassEquality(pStorageClass, vStorageClassObj.(*v1.StorageClass))
		if updatedStorageClass != nil {
			_, err := tenantClient.StorageV1().StorageClasses().Update(updatedStorageClass)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
