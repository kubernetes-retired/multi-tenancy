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
