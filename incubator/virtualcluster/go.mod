module github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster

go 1.12

require (
	github.com/aliyun/alibaba-cloud-sdk-go v1.60.324
	github.com/emicklei/go-restful v2.9.6+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.0
	github.com/google/cadvisor v0.34.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.uber.org/zap v1.9.1
	golang.org/x/crypto v0.0.0-20191206172530-e9b2fee46413 // indirect
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	k8s.io/api v0.16.6
	k8s.io/apimachinery v0.16.6
	k8s.io/apiserver v0.16.6
	k8s.io/client-go v0.16.6
	k8s.io/component-base v0.16.6
	k8s.io/cri-api v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.16.6
	k8s.io/utils v0.0.0-20190829053155-3a4a5477acf8
	sigs.k8s.io/controller-runtime v0.4.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible
	k8s.io/api => k8s.io/api v0.16.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.6
	k8s.io/apiserver => k8s.io/apiserver v0.16.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.6
	k8s.io/client-go => k8s.io/client-go v0.16.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.6
	k8s.io/code-generator => k8s.io/code-generator v0.16.6
	k8s.io/component-base => k8s.io/component-base v0.16.6
	k8s.io/cri-api => k8s.io/cri-api v0.16.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20200117000206-3f805ead6cdb
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.6
	k8s.io/kubectl => k8s.io/kubectl v0.16.6
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20200117002354-7275ea7ddf81
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190819145509-592c9a46fd00
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190819143841-305e1cef1ab1
	k8s.io/node-api => k8s.io/node-api v0.0.0-20190819145652-b61681edbd0a
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190819143045-c84c31c165c4
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20200117001934-76e3585bc069
	k8s.io/sample-controller => k8s.io/sample-controller v0.16.6
)
