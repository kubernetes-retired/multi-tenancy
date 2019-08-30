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
	"crypto/tls"
	"net/http"

	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/klog"
)

// InstallHandlers set router and handlers.
func (s *Server) InstallHandlers() {
	ws := new(restful.WebService)
	ws.Path("/pods").
		Produces(restful.MIME_JSON)
	ws.Route(ws.GET("").
		To(s.proxy).
		Operation("getPods"))
	s.restfulCont.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/run")
	ws.Route(ws.POST("/{podNamespace}/{podID}/{containerName}").
		To(s.proxy).
		Operation("getRun"))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.proxy).
		Operation("getRun"))
	s.restfulCont.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/logs/")
	ws.Route(ws.GET("").
		To(s.proxy).
		Operation("getLogs"))
	ws.Route(ws.GET("/{logpath:*}").
		To(s.proxy).
		Operation("getLogs").
		Param(ws.PathParameter("logpath", "path to the log").DataType("string")))
	s.restfulCont.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/containerLogs")
	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.proxy).
		Operation("getContainerLogs"))
	s.restfulCont.Add(ws)
}

func (s *Server) proxy(req *restful.Request, resp *restful.Response) {
	klog.V(4).Infof("request %+v", req)

	req.Request.URL.Host = s.config.KubeletServerHost
	req.Request.URL.Scheme = "https"

	// there must be a peer certificate in the tls connection
	if req.Request.TLS == nil || len(req.Request.TLS.PeerCertificates) == 0 {
		resp.ResponseWriter.WriteHeader(http.StatusForbidden)
		return
	}
	tenantName := req.Request.TLS.PeerCertificates[0].Subject.CommonName
	TranslatePath(req, tenantName)

	klog.V(4).Infof("request after translate %+v", req.Request.URL)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{s.config.KubeletClientCert},
		},
	}

	handler := proxy.NewUpgradeAwareHandler(req.Request.URL, transport /*transport*/, false /*wrapTransport*/, false /*upgradeRequired*/, &responder{})
	handler.ServeHTTP(resp.ResponseWriter, req.Request)
}

type responder struct{}

func (r *responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	klog.Errorf("Error while proxying request: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
