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

package tenantnamespace

import (
	"fmt"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	tenant2 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/controller/tenant"
)

var c client.Client

const timeout = time.Second * 5

func testCreateTenantNamespaceNoPrefix(c client.Client, g *gomega.GomegaWithT, t *testing.T, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("ta-admin", uniqueNo),
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tns", uniqueNo),
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	var expectedTenantRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantRequest)))

	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))

	nskey := types.NamespacedName{Name: instance.Spec.Name}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	c.Delete(context.TODO(), instance)
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// We should wait until the tenantnamespace cr is deleted
	tns := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, instance) }, timeout).Should(gomega.HaveOccurred())

	c.Delete(context.TODO(), tenant)
}

func testCreateTenantNamespaceWithPrefix(c client.Client, g *gomega.GomegaWithT, t *testing.T, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("ta-admin", uniqueNo),
			RequireNamespacePrefix:   true,
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tns", uniqueNo),
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	var expectedTenantRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantRequest)))

	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))

	nskey := types.NamespacedName{Name: fmt.Sprintf("%+v-%+v", tenant.Spec.TenantAdminNamespaceName, instance.Spec.Name)}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	c.Delete(context.TODO(), instance)
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// We should wait until the tenantnamespace cr is deleted
	tns := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, instance) }, timeout).Should(gomega.HaveOccurred())

	c.Delete(context.TODO(), tenant)

}

func testCreateTenantNamespaceWithPrefixNoSpec(c client.Client, g *gomega.GomegaWithT, t *testing.T, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("ta-admin", uniqueNo),
			RequireNamespacePrefix:   true,
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	var expectedTenantRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantRequest)))

	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))

	nskey := types.NamespacedName{Name: fmt.Sprintf("%+v-%+v", tenant.Spec.TenantAdminNamespaceName, instance.ObjectMeta.Name)}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	c.Delete(context.TODO(), instance)
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// We should wait until the tenantnamespace cr is deleted
	tns := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, instance) }, timeout).Should(gomega.HaveOccurred())

	c.Delete(context.TODO(), tenant)
}

func testImportExistingNamespace(c client.Client, g *gomega.GomegaWithT, t *testing.T, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("ta-admin", uniqueNo),
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tns", uniqueNo),
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	var expectedTenantRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantRequest)))

	// Create t2, make it available before creating tenant namespace object
	t2Ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Spec.Name,
		},
	}
	err = c.Create(context.TODO(), t2Ns)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}

	// Create the TenantNamespace object and expect 1) the Reconcile and 2) ownerReference is added to t2
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// Refresh instance
	err = c.Get(context.TODO(), expectedTenantNSRequest.NamespacedName, instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to get tenant namespace object, got an invalid object error: %v", err)
		return
	}
	// Refresh t2
	nskey := types.NamespacedName{Name: instance.Spec.Name}
	err = c.Get(context.TODO(), nskey, t2Ns)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to get namespace object, got an invalid object error: %v", err)
		return
	}
	expectedOwnerRef := metav1.OwnerReference{
		APIVersion: tenancyv1alpha1.SchemeGroupVersion.String(),
		Kind:       "TenantNamespace",
		Name:       instance.Name,
		UID:        instance.UID,
	}
	g.Expect(len(t2Ns.OwnerReferences) == 1 && expectedOwnerRef == t2Ns.OwnerReferences[0]).To(gomega.BeTrue())

	c.Delete(context.TODO(), instance)
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// We should wait until the tenantnamespace cr is deleted
	tns := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, instance) }, timeout).Should(gomega.HaveOccurred())

	c.Delete(context.TODO(), tenant)
}

func testRoleAndBindingsWithValidAdmin(t *testing.T, g *gomega.GomegaWithT, c client.Client, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	//create tenant-admin
	sa := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("tenant-admin-sa", uniqueNo),
			Namespace: "default",
		},
	}
	err := c.Create(context.TODO(), &sa)
	if err != nil {
		t.Logf("Failed to create tenant admin: %+v with error: %+v", sa.ObjectMeta.Name, err)
		return
	}
	//	defer c.Delete(context.TODO(), &sa)

	//create tenant object
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("tenant-admin-ns", uniqueNo),
			TenantAdmins: []v1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      sa.ObjectMeta.Name,
					Namespace: sa.ObjectMeta.Namespace,
				},
			},
		},
	}
	err = c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedRequestTenant = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestTenant)))
	//	defer c.Delete(context.TODO(), tenant)

	//check admin namespace of tenant is created or not
	tenantadminkey := types.NamespacedName{Name: tenant.Spec.TenantAdminNamespaceName}
	tenantAdminNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), tenantadminkey, tenantAdminNs) }, timeout).
		Should(gomega.Succeed())

	saSecretName, err := findSecretNameOfSA(c, sa.ObjectMeta.Name)
	if err != nil {
		t.Logf("Failed to get secret name of a service account: error: %+v", err)
		return
	}

	//Get secret
	saSecretKey := types.NamespacedName{Name: saSecretName, Namespace: sa.ObjectMeta.Namespace}
	tenantAdminSecret := corev1.Secret{}
	err = c.Get(context.TODO(), saSecretKey, &tenantAdminSecret)
	if err != nil {
		t.Logf("Failed to get tenant admin secret, error %+v", err)
		return
	}

	//Generate user config string
	userCfgStr, err := GenerateCfgStr("kind-kind", cfg.Host, tenantAdminSecret.Data["ca.crt"], tenantAdminSecret.Data["token"], sa.ObjectMeta.Name)
	userCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(userCfgStr))
	if err != nil {
		t.Logf("failed to create user config, got an invalid object error: %v", err)
		return
	}

	//User manager and client
	userMgr, err := manager.New(userCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	userCl := userMgr.GetClient()

	stopUserMgr, userMgrStopped := StartTestManager(userMgr, g)

	defer func() {
		close(stopUserMgr)
		userMgrStopped.Wait()
	}()

	//create tenantnamespace object using user client
	tenantnamespaceObj := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo-tenantns", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tenantnamespace-t", uniqueNo),
		},
	}
	err = userCl.Create(context.TODO(), tenantnamespaceObj)
	if err != nil {
		t.Logf("failed to create tenantnamespace object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenantnamespaceObj.ObjectMeta.Name, Namespace: tenantnamespaceObj.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))

	//check if tenantnamespace is created or not
	nskey := types.NamespacedName{Name: tenantnamespaceObj.Spec.Name}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	//deleting all resources
	c.Delete(context.TODO(), &sa)
	userCl.Delete(context.TODO(), tenantnamespaceObj)

	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	// We should wait until the tenantnamespace cr is deleted
	tns := types.NamespacedName{Name: tenantnamespaceObj.Name, Namespace: tenantnamespaceObj.Namespace}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, tenantnamespaceObj) }, timeout).Should(gomega.HaveOccurred())

	c.Delete(context.TODO(), tenant)

}

func testRoleAndBindingsWithNonValidAdmin(t *testing.T, g *gomega.GomegaWithT, c client.Client, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	//create normal user
	usr := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("fake-sa", uniqueNo),
			Namespace: "default",
		},
	}
	err := c.Create(context.TODO(), &usr)
	if err != nil {
		t.Logf("Failed to create fake user: %+v with error: %+v", usr.ObjectMeta.Name, err)
		return
	}

	//create tenant-admin
	sa := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("tenant-admin-sa", uniqueNo),
			Namespace: "default",
		},
	}
	err = c.Create(context.TODO(), &sa)
	if err != nil {
		t.Logf("Failed to create tenant admin: %+v with error: %+v", sa.ObjectMeta.Name, err)
		return
	}

	//create tenant object
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("tenant-admin-ns", uniqueNo),
			TenantAdmins: []v1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      sa.ObjectMeta.Name,
					Namespace: sa.ObjectMeta.Namespace,
				},
			},
		},
	}
	err = c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedRequestTenant = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestTenant)))

	//check admin namespace of tenant is created or not
	tenantadminkey := types.NamespacedName{Name: tenant.Spec.TenantAdminNamespaceName}
	tenantAdminNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), tenantadminkey, tenantAdminNs) }, timeout).
		Should(gomega.Succeed())

	// get secretname of fake user
	saSecretName, err := findSecretNameOfSA(c, usr.ObjectMeta.Name)
	if err != nil {
		t.Logf("Failed to get secret name of a service account: error: %+v", err)
		return
	}

	//Get secret
	saSecretKey := types.NamespacedName{Name: saSecretName, Namespace: usr.ObjectMeta.Namespace}
	fakeUserSecret := corev1.Secret{}
	err = c.Get(context.TODO(), saSecretKey, &fakeUserSecret)
	if err != nil {
		t.Logf("Failed to get tenant admin secret, error %+v", err)
		return
	}

	//Generate user config string
	userCfgStr, err := GenerateCfgStr("kind-kind", cfg.Host, fakeUserSecret.Data["ca.crt"], fakeUserSecret.Data["token"], usr.ObjectMeta.Name)
	userCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(userCfgStr))
	if err != nil {
		t.Logf("failed to create user config, got an invalid object error: %v", err)
		return
	}

	//User manager and client
	userMgr, err := manager.New(userCfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	userCl := userMgr.GetClient()

	stopUserMgr, userMgrStopped := StartTestManager(userMgr, g)

	defer func() {
		close(stopUserMgr)
		userMgrStopped.Wait()
	}()

	//create tenantnamespace object using user client
	tenantnamespaceObj := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo-tenantns", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tenantnamespace", uniqueNo),
		},
	}

	g.Eventually(func() error { return userCl.Create(context.TODO(), tenantnamespaceObj) }).ShouldNot(gomega.Succeed())

	c.Delete(context.TODO(), &sa)
	c.Delete(context.TODO(), &usr)
	c.Delete(context.TODO(), tenant)
}

func testTenantCleanup(c client.Client, g *gomega.GomegaWithT, t *testing.T, requestsTenant, requestsTenantNS chan reconcile.Request, uniqueNo string) {
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: getUniqueName("tenant-a", uniqueNo),
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: getUniqueName("ta-admin", uniqueNo),
			RequireNamespacePrefix:   true,
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueName("foo", uniqueNo),
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: getUniqueName("tns", uniqueNo),
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	var expectedTenantRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: tenant.ObjectMeta.Name}}
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantRequest)))

	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	var expectedTenantNSRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}}
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))

	nskey := types.NamespacedName{Name: fmt.Sprintf("%+v-%+v", tenant.Spec.TenantAdminNamespaceName, instance.Spec.Name)}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	// Delete tenant should trigger deleting tenantnamespace CR.
	c.Delete(context.TODO(), tenant)
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedTenantNSRequest)))
	tns := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	// We have to clean the block channel to allow the follow up reconcile happens.
PoolChan:
	for true {
		select {
		case <-requestsTenantNS:
		case <-time.After(2 * time.Second):
			break PoolChan
		}
	}
	g.Eventually(func() error { return c.Get(context.TODO(), tns, instance) }, timeout).Should(gomega.HaveOccurred())
}

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	//start tenant controller
	recFnTenant, requestsTenant := tenant2.SetupTestReconcile(tenant2.SetupNewReconciler(mgr))
	g.Expect(tenant2.AddManager(mgr, recFnTenant)).NotTo(gomega.HaveOccurred())

	//start tenantnamespace controller
	recFnTenantNS, requestsTenantNS := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFnTenantNS)).NotTo(gomega.HaveOccurred())

	//start and defer manager
	stopMgr, mgrStopped := StartTestManager(mgr, g)
	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	testCreateTenantNamespaceNoPrefix(c, g, t, requestsTenant, requestsTenantNS, "1")
	testCreateTenantNamespaceWithPrefix(c, g, t, requestsTenant, requestsTenantNS, "2")
	testCreateTenantNamespaceWithPrefixNoSpec(c, g, t, requestsTenant, requestsTenantNS, "3")
	testImportExistingNamespace(c, g, t, requestsTenant, requestsTenantNS, "4")
	testRoleAndBindingsWithValidAdmin(t, g, c, requestsTenant, requestsTenantNS, "5")
	testRoleAndBindingsWithNonValidAdmin(t, g, c, requestsTenant, requestsTenantNS, "6")
	testTenantCleanup(c, g, t, requestsTenant, requestsTenantNS, "7")
}
