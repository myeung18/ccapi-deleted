module github.com/cockroachdb/ccapi-k8s-operator

go 1.16

require (
	github.com/RHEcosystemAppEng/dbaas-operator v0.1.3
	github.com/go-logr/logr v0.4.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/parnurzeal/gorequest v0.2.16
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	moul.io/http2curl v1.0.0 // indirect
	sigs.k8s.io/controller-runtime v0.10.0
)
