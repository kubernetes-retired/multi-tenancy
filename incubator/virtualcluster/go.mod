module sigs.k8s.io/multi-tenancy/incubator/virtualcluster

go 1.13

require (
	github.com/aliyun/alibaba-cloud-sdk-go v1.60.324
	github.com/emicklei/go-restful v2.9.6+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.0
	github.com/google/cadvisor v0.35.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.uber.org/zap v1.10.0
	golang.org/x/net v0.0.0-20200520004742-59133d7f0dd7
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/apiserver v0.18.6
	k8s.io/cli-runtime v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/code-generator v0.18.6
	k8s.io/component-base v0.18.6
	k8s.io/cri-api v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.6
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/controller-runtime v0.6.1
)

// We use the replace directive to pin k8s.io dependencies that we don't directly
// depend on here to the same version as the k8s.io/kubernetes dependency above.
// This is neccessary as k8s.io/kubernetes depends on v0.0.0 of each staging,
// and itself makes use of replace directives (that will not work here) to make
// use of the 'staging' versions of dependencies within the repository.
// TODO: ideally, we should not depend on k8s.io/kubernetes at all, which in turn
//  will avoid us needing to pin dependencies here that we don't actually directly
//  depend on. This is a product of Kubernetes' staging hacks in the main repo,
//  and it is not advised that external projects depend upon k8s.io/kubernetes for
//  this exact reason.
replace (
	k8s.io/api => k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.6
	k8s.io/apiserver => k8s.io/apiserver v0.18.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.6
	k8s.io/client-go => k8s.io/client-go v0.18.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.6
	k8s.io/code-generator => k8s.io/code-generator v0.18.6
	k8s.io/component-base => k8s.io/component-base v0.18.6
	k8s.io/cri-api => k8s.io/cri-api v0.18.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.6
	k8s.io/kubectl => k8s.io/kubectl v0.18.6
	k8s.io/kubelet => k8s.io/kubelet v0.18.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.6
	k8s.io/metrics => k8s.io/metrics v0.18.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.6
)
