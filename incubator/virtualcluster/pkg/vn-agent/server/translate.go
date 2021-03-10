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
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/emicklei/go-restful"
	"k8s.io/klog"
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

// translateRawQuery translates the rawquery for super apiserver
func translateRawQuery(req *restful.Request, containerName string) {
	vals := req.Request.URL.Query()
	query := url.Values{}
	for k, v := range vals {
		switch k {
		case "command":
			query.Add("command", v[0])
		case "follow":
			if v[0] == "true" {
				query.Add("follow", "true")
			}
		case "input":
			if v[0] == "1" {
				query.Add("stdin", "true")
			}
			if v[0] == "0" {
				query.Add("stdin", "false")
			}
		case "output":
			if v[0] == "1" {
				query.Add("stdout", "true")
			}
			if v[0] == "0" {
				query.Add("stdout", "false")
			}
		case "tty":
			if v[0] == "1" {
				query.Add("tty", "true")
			}
			if v[0] == "0" {
				query.Add("stdout", "false")
			}
		default:
			klog.Errorf("unknown rawquery: %s", k)
		}
	}
	if containerName != "" {
		query.Add("container", containerName)
	}
	req.Request.URL.RawQuery = query.Encode()
}

// TranslatePathForSuper translates the URL path to kubelet to super apiserver
func TranslatePathForSuper(req *restful.Request, tenantName string) error {
	action := strings.Split(req.Request.URL.Path[1:], "/")[0]
	var apiserverPath string
	// req.PathParameter inclouding containerName, podID, podNamespace
	pathParas := req.PathParameters()
	podNamespace := pathParas["podNamespace"]
	podID := pathParas["podID"]
	containerName := pathParas["containerName"]
	commonPath := fmt.Sprintf("/api/v1/namespaces/%s-%s/pods/%s", tenantName, podNamespace, podID)

	switch action {
	case "containerLogs":
		// eg. 	/containerLogs/{podNamespace}/{podID}/{containerName}
		// to   /api/v1/namespaces/{tenantName}-{podNamespace}/pods/{podID}/log
		apiserverPath = path.Join(commonPath, "log")
		translateRawQuery(req, containerName)
	case "exec":
		// eg. /exec/{podNamespace}/podID/{containerName}
		// to  /api/v1/namespaces/{tenantName}-{podNamespace}/pods/{podID}/exec
		apiserverPath = path.Join(commonPath, "exec")
		translateRawQuery(req, containerName)
	case "attach":
		apiserverPath = path.Join(commonPath, "attach")
		translateRawQuery(req, containerName)
	case "portForward":
		apiserverPath = path.Join(commonPath, "portForward")
		translateRawQuery(req, "")
	default:
		return fmt.Errorf("unsupport action %s", action)
	}
	req.Request.URL.Path = apiserverPath
	return nil
}
