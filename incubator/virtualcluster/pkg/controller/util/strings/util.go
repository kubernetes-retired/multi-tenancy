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

package strings

import "encoding/json"

// ContainString checks if string slice sli contains string s
func ContainString(sli []string, s string) bool {
	for _, str := range sli {
		if str == s {
			return true
		}
	}
	return false
}

// RemoveString removes string s from the string slice sli
func RemoveString(sli []string, s string) (newSli []string) {
	for _, str := range sli {
		if str == s {
			continue
		}
		newSli = append(newSli, str)
	}
	return
}

// IsJSON check whether given s is in json format
func IsJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
