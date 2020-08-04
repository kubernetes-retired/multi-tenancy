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

package event

import (
	"context"
	"fmt"

	pkgerr "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.eventSynced, c.nsSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.upwardEventController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pEvent, err := c.eventLister.Events(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find pEvent %s/%s in controller cache: %v", pNamespace, pName, err)
	}

	clusterName, tenantNS, err := conversion.GetVirtualNamespace(c.nsLister, pNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find ns %s in controller cache: %v", pNamespace, err)
	}
	if clusterName == "" || tenantNS == "" {
		klog.V(4).Infof("drop event %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	tenantClient, err := c.multiClusterEventController.GetClusterClient(clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to create client from cluster %s config", clusterName)
	}

	vInvolvedObjectType, accepted := c.acceptedEventObj[pEvent.InvolvedObject.Kind]
	if !accepted {
		klog.Warningf("unexpected event %+v in uws", pEvent)
		return nil
	}

	vInvolvedObject, err := c.multiClusterEventController.GetByObjectType(clusterName, tenantNS, pEvent.InvolvedObject.Name, vInvolvedObjectType)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("back populate event: failed to find pod %s/%s in cluster %s", tenantNS, pEvent.InvolvedObject.Name, clusterName)
			return nil
		}
		return err
	}

	vEvent := conversion.BuildVirtualEvent(clusterName, pEvent, vInvolvedObject.(metav1.Object))
	_, err = c.multiClusterEventController.Get(clusterName, tenantNS, vEvent.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = tenantClient.CoreV1().Events(tenantNS).Create(context.TODO(), vEvent, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}
