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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8srt "k8s.io/apimachinery/pkg/runtime"
	utilrt "k8s.io/apimachinery/pkg/util/runtime"
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
	defer utilrt.HandleCrash()

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
	glog.Info("createTenant: %#v", obj)
	syncRBACForTenantCRD(obj)
	// TODO create namespace
	// TODO create RBAC inside namespace
}

func (c *Controller) updateTenant(old, obj *tenantsapi.Tenant) {
	glog.Info("updateTenant: %#v", obj)
	syncRBACForTenantCRD(obj)
	// TODO sync namespace
	// TODO sync RBAC inside namespace
}

func (c *Controller) deleteTenant(obj *tenantsapi.Tenant) {
	glog.Info("deleteTenant: %#v", obj)
	deleteRBACForTenantCRD(obj)
	// TODO delete namespace
}

func rbacForTenantCRD(obj *tenantsapi.Tenant) []k8srt.Object {
	name := fmt.Sprintf("tenant-admins-%s", obj.Name)
	return []k8srt.Object{
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:         []string{"get", "update", "patch", "delete"},
					APIGroups:     []string{tenantsapi.SchemeGroupVersion.Group},
					Resources:     []string{"tenants"},
					ResourceNames: []string{obj.Name},
				},
			},
		}, &rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRoleBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Subjects: obj.Spec.Admins,
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     name,
			},
		},
	}
}

func syncRBACForTenantCRD(obj *tenantsapi.Tenant) {
	if err := newKubeCtl().addObjects(rbacForTenantCRD(obj)...).apply(); err != nil {
		glog.Errorf("syncRBACForTenantCRD error: %v", err)
		// TODO retry logic.
	}
}

func deleteRBACForTenantCRD(obj *tenantsapi.Tenant) {
	if err := newKubeCtl().addObjects(rbacForTenantCRD(obj)...).delete(); err != nil {
		glog.Errorf("deleteRBACForTenantCRD error: %v", err)
		// TODO retry logic.
	}
}
