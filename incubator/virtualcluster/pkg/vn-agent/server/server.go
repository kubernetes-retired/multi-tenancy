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
	"net/http"
	"net/url"

	"github.com/emicklei/go-restful"
	"github.com/pkg/errors"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
)

// Server is a http.Handler which exposes vn-agent functionality over HTTP.
type Server struct {
	config      *config.Config
	restfulCont *restful.Container
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

	server := &Server{
		restfulCont: restful.NewContainer(),
		config:      cfg,
	}

	server.InstallHandlers()
	return server, nil
}
