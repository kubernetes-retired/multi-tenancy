package unittestutils

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis"
	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	tenant "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/controller/tenant"
	tenantnamespace "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/controller/tenantnamespace"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	podutil "k8s.io/kubernetes/test/e2e/framework/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	cfg *rest.Config
	c client.Client
	err error
	timeout = time.Second * 40
	saNamespace = "default"
	tenantName = "tenantA"
	tenantNamespaceName = "tenantnamespaceA"
	actualTenantNamespaceName = "tA-nsA"
)

// In future if we want to add more tenants and tenantnamespaces
var Tenants []*tenancyv1alpha1.Tenant
var Tenantnamespaces []*tenancyv1alpha1.TenantNamespace
var ServiceAccounts []*corev1.ServiceAccount


// ServiceAccountObj returns the pointer to a service account object
func ServiceAccountObj(name string, namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// TenantObj returns the pointer to a tenant object
func TenantObj(name string, sa *corev1.ServiceAccount, namespace string) *tenancyv1alpha1.Tenant {
	return &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: tenancyv1alpha1.TenantSpec{
			TenantAdminNamespaceName: namespace,
			TenantAdmins: []v1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      sa.ObjectMeta.Name,
					Namespace: sa.ObjectMeta.Namespace,
				},
			},
		},
	}
}

// TenantNamespaceObj returns the pointer to a TenantNamespace object
func TenantNamespaceObj(name string, adminNamespace string, namespace string) *tenancyv1alpha1.TenantNamespace {
	return &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: adminNamespace,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: namespace,
		},
	}
}

// CreateCrds installs the tenant ,tenantnamespace and kyverno CRDs
func CreateCrds(testClient *TestClient) error {
	tr := true
	apis.AddToScheme(scheme.Scheme)

	e := &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "..", "assets")},
		UseExistingCluster: &tr,
	}

	if cfg, err = e.Start(); err != nil {
		return err
	}
	e.Stop()

	// Install Kyverno
	path := filepath.Join("..", "..", "assets")
	crdPath := filepath.Join(path, "kyverno.yaml")
	err = testClient.CreatePolicy(crdPath)
	if err != nil {
		return err
	}

	err := waitForKyvernoToReady(testClient.K8sClient)
	if err != nil {
		return err
	}

	return nil
}

func waitForKyvernoToReady(k8sClient *kubernetes.Clientset) error {
	var podsList  *corev1.PodList
	for {
		podsList, err = k8sClient.CoreV1().Pods("kyverno").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
		fmt.Println(podsList)
		if(len(podsList.Items) > 0) {
			break
		}
	}
	podNames := []string{podsList.Items[0].ObjectMeta.Name}

	for {
		if podutil.CheckPodsRunningReady(k8sClient, "kyverno", podNames, 200*time.Second) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

// CheckNamespaceExist namespace exists or not
func CheckNamespaceExist(namespace string, k8sClient *kubernetes.Clientset) bool {
	_, err = k8sClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})	
	if err == nil {
		return true
	}
	return false
}

// CreateTenant creates the tenant and tenantnamespace 
func CreateTenant(t *testing.T, g *gomega.WithT) {
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	c = mgr.GetClient()

	recFn, _ := tenant.SetupTestReconcile(tenant.SetupNewReconciler(mgr))
	g.Expect(tenant.AddManager(mgr, recFn)).NotTo(gomega.HaveOccurred())

	recFnTenantNS, _ := tenantnamespace.SetupTestReconcile(tenantnamespace.NewReconciler(mgr))
	g.Expect(tenantnamespace.AddManager(mgr, recFnTenantNS)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := tenant.StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	for _, sa := range ServiceAccounts {
		// Create the service account
		err = c.Create(context.TODO(), sa)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	for _, tenant := range Tenants {
		// Create the Tenant object and expect the tenantAdminNamespace to be created
		err = c.Create(context.TODO(), tenant)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		// Wait for the tenantadminnamespace to be created
		nskey := types.NamespacedName{Name: tenant.Spec.TenantAdminNamespaceName}
		adminNs := &corev1.Namespace{}
		g.Eventually(func() error { return c.Get(context.TODO(), nskey, adminNs) }, timeout).Should(gomega.Succeed())
		// Wait for the tenant roles to be created
		rolekey := types.NamespacedName{Name: "tenant-admin-role", Namespace: tenant.Spec.TenantAdminNamespaceName}
		tenantRole := &v1.Role{}
		g.Eventually(func() error { return c.Get(context.TODO(), rolekey, tenantRole) }, timeout).Should(gomega.Succeed())
	}

	for _, tenantnamespaceObj := range Tenantnamespaces {
		// Create the tenant namespace obj
		err = c.Create(context.TODO(), tenantnamespaceObj)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		//check if tenantnamespace is created or not
		nskey := types.NamespacedName{Name: tenantnamespaceObj.Spec.Name}
		tenantNs := &corev1.Namespace{}
		g.Eventually(func() error { return c.Get(context.TODO(), nskey, tenantNs) }, timeout).
			Should(gomega.Succeed())
	}

}
