package unittestutils

import (
	"context"
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
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var cfg *rest.Config
var c client.Client
var err error

const timeout = time.Second * 40

// In future if we want to add more tenants and tenantnamespaces
var tenants []*tenancyv1alpha1.Tenant
var tenantnamespaces []*tenancyv1alpha1.TenantNamespace
var serviceAccount []*corev1.ServiceAccount

type RoleRuleOwner struct {
	Role *v1.Role
}

func CreateCrds() error {
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

	return err
}

func CreateTenant(t *testing.T, g *gomega.WithT, namespace string, serviceAcc string) {

	var sa = &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAcc,
			Namespace: "default",
		},
	}

	serviceAccount = append(serviceAccount, sa)

	var tenant1 = &tenancyv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant1-sample",
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

	tenants = append(tenants, tenant1)

	var tenantnamespaceOne = &tenancyv1alpha1.TenantNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant1namespace-sample",
			Namespace: tenant1.Spec.TenantAdminNamespaceName,
		},
		Spec: tenancyv1alpha1.TenantNamespaceSpec{
			Name: "t1-ns1",
		},
	}
	tenantnamespaces = append(tenantnamespaces, tenantnamespaceOne)

	kubecfgFlags := genericclioptions.NewConfigFlags(false)	// Create the K8s clientSet
	cfg, err := kubecfgFlags.ToRESTConfig()
	k8sClient, err := kubernetes.NewForConfig(cfg)
	_, err = k8sClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})	
	if err == nil {
		return
	}


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

	for _, sa := range serviceAccount {
		// Create the service account
		err = c.Create(context.TODO(), sa)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	for _, tenant := range tenants {
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

	for _, tenantnamespaceObj := range tenantnamespaces {
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

func DestroyTenantSA(g *gomega.WithT) {
	// // Delete Service Account
	// fmt.Println("Deleting service accounts")
	// for _, sa := range serviceAccount {
	// 	err = c.Delete(context.TODO(), sa)
	// 	g.Expect(err).NotTo(gomega.HaveOccurred())
	// }

	// // Delete Tenant
	// fmt.Println("Deleting tenants")
	// for _, tenant := range tenants {
	// 	err = c.Delete(context.TODO(), tenant)
	// 	g.Expect(err).NotTo(gomega.HaveOccurred())
	// }

	// // Delete TenantNamespace
	// fmt.Println("Deleting tenant namespaces")
	// for _, tenantnamespace := range tenantnamespaces {
	// 	err = c.Delete(context.TODO(), tenantnamespace)
	// 	g.Expect(err).NotTo(gomega.HaveOccurred())
	// }
}
