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
	tenant2 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/controller/tenant"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/tools/clientcmd"
	"strings"

	//"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

const timeout = time.Second * 5

func testCreateTenantNamespaceNoPrefix(c client.Client, g *gomega.GomegaWithT, t *testing.T, requests chan reconcile.Request) {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "ta-admin"}}
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "ta-admin",
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "ta-admin",
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "t1",
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), tenant)
	// Tenant reconcile is not active hence we need to manually create tenant admin namespace
	adminNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ta-admin",
		},
	}
	err = c.Create(context.TODO(), adminNs)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), adminNs)
	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	nskey := types.NamespacedName{Name: "t1"}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	// Delete the namespace and expect reconcile to be called to create the namespace again
	// XXX: ns cannot be deleted in Test APIserver for some reason, comment out the test for now

	// g.Expect(c.Delete(context.TODO(), tenantNs)).NotTo(gomega.HaveOccurred())
	// g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
	//	Should(gomega.Succeed())
}

func testCreateTenantNamespaceWithPrefix(c client.Client, g *gomega.GomegaWithT, t *testing.T, requests chan reconcile.Request) {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-1", Namespace: "ta-admin"}}
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "ta-admin",
			RequireNamespacePrefix:   true,
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-1",
			Namespace: "ta-admin",
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "t1",
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), tenant)
	// Tenant reconcile is not active hence we need to manually create tenant admin namespace
	adminNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ta-admin",
		},
	}
	err = c.Create(context.TODO(), adminNs)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), adminNs)
	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	nskey := types.NamespacedName{Name: "ta-admin-t1"}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())
}

func testCreateTenantNamespaceWithPrefixNoSpec(c client.Client, g *gomega.GomegaWithT, t *testing.T, requests chan reconcile.Request) {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-2", Namespace: "ta-admin"}}
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "ta-admin",
			RequireNamespacePrefix:   true,
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-2",
			Namespace: "ta-admin",
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), tenant)
	// Tenant reconcile is not active hence we need to manually create tenant admin namespace
	adminNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ta-admin",
		},
	}
	err = c.Create(context.TODO(), adminNs)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), adminNs)
	// Create the TenantNamespace object and expect the Reconcile and the namespace to be created
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	nskey := types.NamespacedName{Name: "ta-admin-foo-2"}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())
}

func testImportExistingNamespace(c client.Client, g *gomega.GomegaWithT, t *testing.T, requests chan reconcile.Request) {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-3", Namespace: "ta-admin"}}
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "ta-admin",
		},
	}
	instance := &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-3",
			Namespace: "ta-admin",
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "t2",
		},
	}
	// Create tenant object
	err := c.Create(context.TODO(), tenant)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create tenant object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), tenant)
	// Tenant reconcile is not active hence we need to manually create tenant admin namespace
	adminNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ta-admin",
		},
	}
	err = c.Create(context.TODO(), adminNs)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), adminNs)
	// Create t2, make it available before creating tenant namespace object
	t2Ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "t2",
		},
	}
	err = c.Create(context.TODO(), t2Ns)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create namespace object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), t2Ns)
	// Create the TenantNamespace object and expect 1) the Reconcile and 2) ownerReference is added to t2
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// Refresh instance
	err = c.Get(context.TODO(), expectedRequest.NamespacedName, instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to get tenant namespace object, got an invalid object error: %v", err)
		return
	}
	// Refresh t2
	nskey := types.NamespacedName{Name: "t2"}
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
}

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	testCreateTenantNamespaceNoPrefix(c, g, t, requests)
	testCreateTenantNamespaceWithPrefix(c, g, t, requests)
	testCreateTenantNamespaceWithPrefixNoSpec(c, g, t, requests)
	testImportExistingNamespace(c, g, t, requests)
}

func TestBothReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testRoleAndBindingsWithValidAdmin(t, g)
	testRoleAndBindingsWithNonValidAdmin(t, g)
}

func testRoleAndBindingsWithValidAdmin(t *testing.T, g *gomega.GomegaWithT) {
	var expectedRequestTenantNamespace = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-tenantns", Namespace: "tenant-admin-ns"}}
	var expectedRequestTenant = reconcile.Request{NamespacedName: types.NamespacedName{Name: "tenant-a"}}

	//setup and client
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

	//create tenant-admin
	sa := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-admin-sa",
			Namespace: "default",
		},
	}
	err = c.Create(context.TODO(), &sa)
	if err != nil {
		t.Logf("Failed to create tenant admin: %+v with error: %+v", sa.ObjectMeta.Name, err)
		return
	}
	//defer c.Delete(context.TODO(), &sa)

	//create tenant object
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "tenant-admin-ns",
			TenantAdmins: []v1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "tenant-admin-sa",
					Namespace: "default",
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
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestTenant)))
	//defer c.Delete(context.TODO(), tenant)

	//check admin namespace of tenant is created or not
	tenantadminkey := types.NamespacedName{Name: "tenant-admin-ns"}
	tenantAdminNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), tenantadminkey, tenantAdminNs) }, timeout).
		Should(gomega.Succeed())

	//Get service account list, to fetch above sa because secret generation is async
	saList := corev1.ServiceAccountList{}
	if err = c.List(context.TODO(), &client.ListOptions{}, &saList); err != nil {
		t.Logf("Failed to get serciceaccountlist, error %+v", err)
		return
	}

	//Get secret name of tenant admin service account
	var saSecretName string
	for _, eachSA := range saList.Items {
		for _, each := range eachSA.Secrets {
			if strings.Contains(each.Name, sa.ObjectMeta.Name) {
				saSecretName = each.Name
			}
		}
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
			Name:      "foo-tenantns",
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "tenantnamespace",
		},
	}
	err = userCl.Create(context.TODO(), tenantnamespaceObj)
	if err != nil {
		t.Logf("failed to create tenantnamespace object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requestsTenantNS, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestTenantNamespace)))
	//defer userCl.Delete(context.TODO(), tenantnamespaceObj)

	nskey := types.NamespacedName{Name: "tenantnamespace"}
	tenantNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
		Should(gomega.Succeed())

	c.Delete(context.TODO(), &sa)
	c.Delete(context.TODO(), tenant)
	c.Delete(context.TODO(), tenantAdminNs)
	userCl.Delete(context.TODO(), tenantnamespaceObj)
}

func testRoleAndBindingsWithNonValidAdmin(t *testing.T, g *gomega.GomegaWithT) {
	var expectedRequestTenant = reconcile.Request{NamespacedName: types.NamespacedName{Name: "tenant-a-2"}}

	//setup and client
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	//start tenant controller
	recFnTenant, requestsTenant := tenant2.SetupTestReconcile(tenant2.SetupNewReconciler(mgr))
	g.Expect(tenant2.AddManager(mgr, recFnTenant)).NotTo(gomega.HaveOccurred())

	//start tenantnamespace controller
	recFnTenantNS, _ := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFnTenantNS)).NotTo(gomega.HaveOccurred())

	//start and defer manager
	stopMgr, mgrStopped := StartTestManager(mgr, g)
	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	//create normal user
	usr := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-sa-2",
			Namespace: "default",
		},
	}
	err = c.Create(context.TODO(), &usr)
	if err != nil {
		t.Logf("Failed to create fake user: %+v with error: %+v", usr.ObjectMeta.Name, err)
		return
	}
	defer c.Delete(context.TODO(), &usr)

	//create tenant-admin
	sa := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-admin-sa-2",
			Namespace: "default",
		},
	}
	err = c.Create(context.TODO(), &sa)
	if err != nil {
		t.Logf("Failed to create tenant admin: %+v with error: %+v", sa.ObjectMeta.Name, err)
		return
	}
	defer c.Delete(context.TODO(), &sa)

	//create tenant object
	tenant := &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-a-2",
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: "tenant-admin-ns-2",
			TenantAdmins: []v1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "tenant-admin-sa-2",
					Namespace: "default",
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
	g.Eventually(requestsTenant, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestTenant)))
	defer c.Delete(context.TODO(), tenant)

	//check admin namespace of tenant is created or not
	tenantadminkey := types.NamespacedName{Name: "tenant-admin-ns-2"}
	tenantAdminNs := &corev1.Namespace{}
	g.Eventually(func() error { return c.Get(context.TODO(), tenantadminkey, tenantAdminNs) }, timeout).
		Should(gomega.Succeed())

	//Get service account list, to fetch above sa because secret generation is async
	saList := corev1.ServiceAccountList{}
	if err = c.List(context.TODO(), &client.ListOptions{}, &saList); err != nil {
		t.Logf("Failed to get serciceaccountlist, error %+v", err)
		return
	}

	//Get secret name of fake user service account
	var saSecretName string
	for _, eachSA := range saList.Items {
		for _, each := range eachSA.Secrets {
			if strings.Contains(each.Name, usr.ObjectMeta.Name) {
				saSecretName = each.Name
			}
		}
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
			Name:      "foo-tenantns-2",
			Namespace: tenant.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "tenantnamespace-2",
		},
	}

	g.Eventually(func() error { return userCl.Create(context.TODO(), tenantnamespaceObj) }).ShouldNot(gomega.Succeed())

	c.Delete(context.TODO(), &sa)
	c.Delete(context.TODO(), &usr)
	c.Delete(context.TODO(), tenant)
	c.Delete(context.TODO(), tenantAdminNs)
}

