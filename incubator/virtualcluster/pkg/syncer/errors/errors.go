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
	"fmt"

	pkgerr "github.com/pkg/errors"
)

const (
	codeClusterNotFound = iota
	codeUnknown
)

// Error is a type of error used for sync request.
type errorType struct {
	code int
	msg  string
}

func (e errorType) Error() string {
	return e.msg
}

var _ error = errorType{}

func reasonForError(err error) int {
	err = pkgerr.Cause(err)
	if t, ok := err.(errorType); ok {
		return t.code
	}
	return codeUnknown
}

// NewClusterNotFound returns an error indicating that the cluster was not found.
func NewClusterNotFound(clusterName string) error {
	return errorType{
		code: codeClusterNotFound,
		msg:  fmt.Sprintf("cluster %s not found", clusterName),
	}
}

// IsClusterNotFound returns true if the specified error was ClusterNotFound.
func IsClusterNotFound(err error) bool {
	return reasonForError(err) == codeClusterNotFound
}
