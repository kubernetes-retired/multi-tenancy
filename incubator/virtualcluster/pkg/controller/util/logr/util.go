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

package logr

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// NewLogger creates a new logr.Logger based on the zap.Logger. If the 'logFile'
// is not empty, log to stderr and the 'logFile', otherwise, log to the stderr
// only. stacktrace indicates whether manager will disable the stacktrace
func NewLogger(logFile string, disableStacktrace bool) (logr.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stderr"}
	// logs to both stderr and 'logFile'
	if logFile != "" {
		cfg.OutputPaths = append(cfg.OutputPaths, logFile)
	}
	// allow user to disable noisy stacktrace
	cfg.DisableStacktrace = disableStacktrace
	// the caller will always be zapr.go, which is useless
	cfg.DisableCaller = true
	zLogr, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return zapr.NewLogger(zLogr), nil
}
