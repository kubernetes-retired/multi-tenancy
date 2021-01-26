/*
Copyright 2021 The Kubernetes Authors.

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

package patrol

import (
	"time"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type OptConfig func(*Options)

// WithOptions set options.
func WithOptions(o *Options) OptConfig {
	return func(options *Options) {
		if o == nil {
			return
		}
		WithControllerName(o.name)(options)
		WithReconciler(o.Reconciler)(options)
		WithPeriod(o.Period)(options)
	}
}

// WithControllerName set the controller name.
func WithControllerName(name string) OptConfig {
	return func(options *Options) {
		if name != "" {
			options.name = name
		}
	}
}

// WithReconciler set the reconciler.
func WithReconciler(rc reconciler.PatrolReconciler) OptConfig {
	return func(options *Options) {
		if rc != nil {
			options.Reconciler = rc
		}
	}
}

// WithPeriod set patrol JitterPeriod.
func WithPeriod(t time.Duration) OptConfig {
	return func(options *Options) {
		if t > 0 {
			options.Period = t
		}
	}
}
