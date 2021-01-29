module sigs.k8s.io/multi-tenancy/incubator/hnc

go 1.14

require (
	contrib.go.opencensus.io/exporter/prometheus v0.2.0
	contrib.go.opencensus.io/exporter/stackdriver v0.13.0
	github.com/emicklei/go-restful v2.10.0+incompatible // indirect
	github.com/go-logr/logr v0.3.0
	github.com/go-openapi/spec v0.19.5 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/open-policy-agent/cert-controller v0.0.0-20210129015139-6ff9721a1c47
	github.com/spf13/cobra v1.0.0
	go.opencensus.io v0.22.3
	go.uber.org/zap v1.15.0
	k8s.io/api v0.19.2
	k8s.io/apiextensions-apiserver v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/cli-runtime v0.19.2
	k8s.io/client-go v0.19.2
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/controller-tools v0.2.8
)
