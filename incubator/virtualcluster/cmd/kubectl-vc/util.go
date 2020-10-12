/*
Copyright 2020 The Kubernetes Authors.

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

package main

import (
	"io/ioutil"
	"net/http"
	"strings"
)

// getYamlContent reads the yaml content from the `yamlPath`
func getYamlContent(yamlPath string) ([]byte, error) {
	if isURL(yamlPath) {
		// read from an URL
		resp, err := http.Get(yamlPath)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		yamlContent, err := ioutil.ReadAll(resp.Body)
		return yamlContent, nil
	}
	// read from a file
	yamlContent, err := ioutil.ReadFile(yamlPath)
	return yamlContent, err
}

// isURL checks if `path` is an URL
func isURL(path string) bool {
	return strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://")
}
