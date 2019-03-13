// Copyright 2017 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tenants

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	apirt "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	tenantsapi "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/apis/tenants/v1alpha1"
	tenantsclient "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/clientset/v1alpha1"
	tenantsinformers "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/informers/externalversions"
)

// Controller is k8s controller managing Tenant CRDs.
type Controller struct {
	informer cache.SharedIndexInformer
}

// NewController creates the controller.
func NewController(client tenantsclient.Interface, informerFactory tenantsinformers.SharedInformerFactory) *Controller {
	c := &Controller{
		informer: informerFactory.Tenants().V1alpha1().Tenants().Informer(),
	}
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(o interface{}) { c.createTenant(o.(*tenantsapi.Tenant)) },
		UpdateFunc: func(o, n interface{}) { c.updateTenant(o.(*tenantsapi.Tenant), n.(*tenantsapi.Tenant)) },
		DeleteFunc: func(o interface{}) { c.deleteTenant(o.(*tenantsapi.Tenant)) },
	})
	return c
}

// Run implements the controller logic.
func (c *Controller) Run(ctx context.Context) error {
	defer apirt.HandleCrash()

	glog.Info("waiting for cache sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("cache sync failed")
	}

	glog.Info("controller started")
	<-ctx.Done()
	glog.Info("controller stopped")

	return nil
}

func (c *Controller) createTenant(obj *tenantsapi.Tenant) {
	// TODO
	glog.Info("createTenant: %#v", obj)
}

func (c *Controller) updateTenant(old, obj *tenantsapi.Tenant) {
	// TODO
	glog.Info("updateTenant: %#v", obj)
}

func (c *Controller) deleteTenant(obj *tenantsapi.Tenant) {
	// TODO
	glog.Info("deleteTenant: %#v", obj)
}
