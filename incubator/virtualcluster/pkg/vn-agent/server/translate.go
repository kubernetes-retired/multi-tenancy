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
	"strings"

	"github.com/emicklei/go-restful"
)

// TranslatePath translate the naming between tenant and master cluster.
func TranslatePath(req *restful.Request, tenantName string) {
	podNamespace := req.PathParameter("podNamespace")
	path := req.Request.URL.Path
	if podNamespace != "" {
		// eg.   /containerLogs/{podNamespace}/{podID}/{containerName}
		//    to /containerLogs/{tenantName}-{podNamespace}/{podID}/{containerName}
		secondSlash := strings.IndexByte(path[1:], '/')
		path = path[:secondSlash+2] + tenantName + "-" + path[secondSlash+2:]
	}
	req.Request.URL.Path = path
}
