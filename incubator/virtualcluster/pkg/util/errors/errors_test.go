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

package errors

import (
	"testing"

	pkgerr "github.com/pkg/errors"
)

func TestErrorCheck(t *testing.T) {
	if !IsClusterNotFound(NewClusterNotFound("test")) {
		t.Error("expected to be ClusterNotFoundError")
	}
	if !IsClusterNotFound(pkgerr.Wrapf(NewClusterNotFound("test"), "nested error")) {
		t.Error("expected to be ClusterNotFoundError")
	}
	if IsClusterNotFound(errorType{-1, "unknown"}) {
		t.Error("expected to not be ClusterNotFoundError")
	}
	if IsClusterNotFound(nil) {
		t.Error("expected to not be ClusterNotFoundError")
	}
}
