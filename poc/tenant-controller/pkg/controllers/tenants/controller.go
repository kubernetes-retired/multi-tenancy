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
	"sort"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8srt "k8s.io/apimachinery/pkg/runtime"
	utilrt "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	tenantsapi "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/apis/tenants/v1alpha1"
	tenantsclient "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/clientset/v1alpha1"
	tenantsinformers "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/informers/externalversions"
)

// Controller is k8s controller managing Tenant CRDs.
type Controller struct {
	informer      cache.SharedIndexInformer
	tenantsclient tenantsclient.Interface
	k8sclient     k8sclient.Interface
}

const (
	defaultAdminRoleBindingName = "admins"
	defaultAdminClusterRole     = "admin"
)

// NewController creates the controller.
func NewController(k8sclient k8sclient.Interface, tenantsclient tenantsclient.Interface, informerFactory tenantsinformers.SharedInformerFactory) *Controller {
	c := &Controller{
		informer:      informerFactory.Tenants().V1alpha1().Tenants().Informer(),
		tenantsclient: tenantsclient,
		k8sclient:     k8sclient,
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
	glog.V(2).Infof("createTenant: %#v", obj)
	c.syncRBACForTenant(obj)
	for _, nsReq := range obj.Spec.Namespaces {
		c.createNamespaceForTenant(obj, &nsReq)
	}
	glog.V(2).Infof("createTenant completed: %#v", obj)
}

func (c *Controller) updateTenant(old, obj *tenantsapi.Tenant) {
	glog.V(2).Infof("updateTenant: %#v", obj)
	c.syncRBACForTenant(obj)
	// sort namespaces in old and new tenants to find out which ones
	// to be created and which ones to be deleted.
	oldNsList := make([]string, len(old.Spec.Namespaces))
	for i, ns := range old.Spec.Namespaces {
		oldNsList[i] = ns.Name
	}
	sort.Strings(oldNsList)
	nsList := make([]*tenantsapi.TenantNamespace, len(obj.Spec.Namespaces))
	for i := range obj.Spec.Namespaces {
		nsList[i] = &obj.Spec.Namespaces[i]
	}
	sort.Slice(nsList, func(i, j int) bool {
		return strings.Compare(nsList[i].Name, nsList[j].Name) < 0
	})
	var i, j int
	for i < len(oldNsList) && j < len(nsList) {
		if res := strings.Compare(oldNsList[i], nsList[j].Name); res == 0 {
			c.syncNamespaceForTenant(obj, nsList[j])
			i++
			j++
		} else if res < 0 {
			c.deleteNamespaceForTenant(obj, oldNsList[i])
			i++
		} else {
			c.createNamespaceForTenant(obj, nsList[j])
			j++
		}
	}

	for ; j < len(nsList); j++ {
		c.createNamespaceForTenant(obj, nsList[j])
	}
	for ; i < len(oldNsList); i++ {
		c.deleteNamespaceForTenant(obj, oldNsList[i])
	}
}

func (c *Controller) deleteTenant(obj *tenantsapi.Tenant) {
	glog.V(2).Infof("deleteTenant: %#v", obj)

	// TODO with OwnerReferences, no extra work is needed in deletion,
	// remove the following code later.

	c.deleteRBACForTenant(obj)
	for _, nsReq := range obj.Spec.Namespaces {
		c.deleteNamespaceForTenant(obj, nsReq.Name)
	}
	glog.Infof("deleteTenant completed: %#v", obj)
}

func (c *Controller) createNamespaceForTenant(tenant *tenantsapi.Tenant, nsReq *tenantsapi.TenantNamespace) error {
	if err := c.ensureNamespaceExists(tenant, nsReq.Name); err != nil {
		return err
	}
	if err := c.syncNamespaceForTenant(tenant, nsReq); err != nil {
		return err
	}
	glog.Infof("Tenant %q namespace %q created", tenant.Name, nsReq.Name)
	return nil
}

func (c *Controller) deleteNamespaceForTenant(tenant *tenantsapi.Tenant, nsName string) error {
	// TODO add a full set of sanity checks in future before deleting
	if err := c.k8sclient.CoreV1().Namespaces().Delete(nsName, nil); err != nil {
		glog.Errorf("Tenant %q delete namespace %q error: %v", tenant.Name, nsName, err)
		return err
	}
	glog.Infof("Tenant %q namespace %q deleted", tenant.Name, nsName)
	return nil
}

func (c *Controller) ensureNamespaceExists(tenant *tenantsapi.Tenant, nsName string) error {
	// TODO Add later ... sanity checks to ensure namespaces being requested are valid and not already assigned to another tenant
	if err := newKubeCtl().addObjects(&corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            nsName,
			OwnerReferences: ownerRefsForTenant(tenant),
		},
	}).apply(); err != nil {
		// TODO add error condition in Tenant object and emit event.
		glog.Errorf("Tenant %q create namespace %q error: %v", tenant.Name, nsName, err)
		return err
	}
	return nil
}

func (c *Controller) syncNamespaceForTenant(tenant *tenantsapi.Tenant, nsReq *tenantsapi.TenantNamespace) error {
	if err := newKubeCtl().withNamespace(nsReq.Name).addObjects(&rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultAdminRoleBindingName,
		},
		Subjects: tenant.Spec.Admins,
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     defaultAdminClusterRole,
		},
	}).apply(); err != nil {
		glog.Errorf("Tenant %q namespace %q sync error: %v", tenant.Name, nsReq.Name, err)
		return err
	}
	return nil
}

func (c *Controller) syncRBACForTenant(tenant *tenantsapi.Tenant) error {
	if err := newKubeCtl().addObjects(rbacForTenant(tenant)...).apply(); err != nil {
		glog.Errorf("Tenant %q syncRBAC error: %v", tenant.Name, err)
		return err
	}
	return nil
}

func (c *Controller) deleteRBACForTenant(tenant *tenantsapi.Tenant) error {
	if err := newKubeCtl().addObjects(rbacForTenant(tenant)...).delete(); err != nil {
		glog.Errorf("Tenant %q deleteRBAC error: %v", tenant.Name, err)
		return err
	}
	return nil
}

func ownerRefsForTenant(tenant *tenantsapi.Tenant) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: tenantsapi.SchemeGroupVersion.String(),
			Kind:       "Tenant",
			Name:       tenant.Name,
			UID:        tenant.UID,
		},
	}
}

func rbacForTenant(tenant *tenantsapi.Tenant) []k8srt.Object {
	name := fmt.Sprintf("tenant-admins-%s", tenant.Name)
	return []k8srt.Object{
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				OwnerReferences: ownerRefsForTenant(tenant),
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:         []string{"get", "update", "patch", "delete"},
					APIGroups:     []string{tenantsapi.SchemeGroupVersion.Group},
					Resources:     []string{"tenants"},
					ResourceNames: []string{tenant.Name},
				},
			},
		}, &rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacv1.SchemeGroupVersion.String(),
				Kind:       "ClusterRoleBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				OwnerReferences: ownerRefsForTenant(tenant),
			},
			Subjects: tenant.Spec.Admins,
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     name,
			},
		},
	}
}
