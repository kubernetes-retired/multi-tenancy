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

package utils

import (
	"math/rand"
	"strings"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// GenAnAvailableName generate a unique valid name against the string slice passed in.
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
func GenAnAvailableName(names []string, prefix string, n int) string {
	nameMap := make(map[string]struct{})
	for _, name := range names {
		nameMap[name] = struct{}{}
	}

	if n < len(prefix) {
		panic("target string length cannot larger than the prefix one")
	}

	for {
		b := make([]rune, n-len(prefix))
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}

		ans := prefix + string(b)

		if _, exists := nameMap[prefix+string(b)]; !exists {
			return ans
		}
	}
}

// HasPrefixs checks whether the string 's' begins with any prefix in 'prefixs'.
func HasPrefixs(s string, prefixs []string) bool {
	for _, pf := range prefixs {
		if strings.HasPrefix(s, pf) {
			return true
		}
	}
	return false
}
