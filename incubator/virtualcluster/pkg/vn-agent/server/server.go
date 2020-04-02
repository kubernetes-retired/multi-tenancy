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

package server

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/emicklei/go-restful"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
)

// Server is a http.Handler which exposes vn-agent functionality over HTTP.
type Server struct {
	superMasterClient client.Client
	config            *config.Config
	restfulCont       *restful.Container
}

// ServeHTTP responds to HTTP requests on the vn-agent.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.restfulCont.ServeHTTP(w, req)
}

// NewServer initializes and configures a vn-agent.Server object to handle HTTP requests.
func NewServer(cfg *config.Config) (*Server, error) {
	u, err := url.Parse(cfg.KubeletServerHost)
	if err != nil {
		return nil, errors.Wrap(err, "parse kubelet server url")
	}
	cfg.KubeletServerHost = u.Host

	// create the super-master client to read Virtualcluster information
	kbCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "fail to create the super-master client")
	}
	cliScheme := runtime.NewScheme()
	smCli, err := client.New(kbCfg, client.Options{Scheme: cliScheme})
	if err != nil {
		return nil, err
	}
	if err = tenancyv1alpha1.AddToScheme(cliScheme); err != nil {
		return nil, errors.Wrap(err, "faill to virtualcluster to the super-master client scheme")
	}

	server := &Server{
		superMasterClient: smCli,
		restfulCont:       restful.NewContainer(),
		config:            cfg,
	}

	server.InstallHandlers()
	return server, nil
}

// GetTenantName finds the virtualcluster with the signature that matches the `reqCrt`
func (s *Server) GetTenantName(reqCrt *x509.Certificate) (string, error) {
	vcLst := &tenancyv1alpha1.VirtualclusterList{}
	if err := s.superMasterClient.List(context.TODO(), vcLst, &client.ListOptions{}); err != nil {
		return "", errors.Wrap(err, "fail to list Virtualclusters on the super master")
	}

	reqSig := reqCrt.Signature

	for _, vc := range vcLst.Items {
		sigB64, exist := vc.Annotations[constants.AnnoX509SignatureBase64]
		if !exist {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			return "", errors.Wrap(err, "fail to decode signature content from the base64 format")
		}
		if bytes.Equal(reqSig, sig) {
			klog.V(4).Infof("received request from vc %s", vc.GetName())
			return conversion.ToClusterKey(&vc), nil
		}
	}
	return "", errors.New("there exist no virtualcluster that matches the signature in the request")
}
