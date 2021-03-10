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

package featuregate

import (
	"fmt"
	"sync"
	"sync/atomic"
)

var DefaultFeatureGate, _ = NewFeatureGate(nil)

// FeatureGate indicates whether a given feature is enabled or not
type FeatureGate interface {
	// Enabled returns true if the key is enabled.
	Enabled(key Feature) bool
	// Set feature gate for known features.
	Set(key Feature, value bool) error
}

const (
	// SuperClusterServiceNetwork is an experimental feature that allows the
	// services to share the same ClusterIPs as the super cluster.
	SuperClusterServiceNetwork = "SuperClusterServiceNetwork"

	// SuperClusterPooling is an experimental feature that allows the syncer to
	// pool multiple super clusters for use with the experimental scheduler
	SuperClusterPooling = "SuperClusterPooling"
)

var defaultFeatures = FeatureList{
	SuperClusterPooling:        {Default: false},
	SuperClusterServiceNetwork: {Default: false},
}

type Feature string

// FeatureSpec represents a feature being gated
type FeatureSpec struct {
	Default bool
}

// FeatureList represents a list of feature gates
type FeatureList map[Feature]FeatureSpec

// Supports indicates whether a feature name is supported on the given
// feature set
func Supports(featureList FeatureList, featureName string) bool {
	for k := range featureList {
		if featureName == string(k) {
			return true
		}
	}
	return false
}

// featureGate implements FeatureGate
type featureGate struct {
	mu sync.Mutex
	// enabled holds a map[Feature]bool
	enabled *atomic.Value
}

// NewFeatureGate stores flag gates for known features from a map[string]bool or returns an error
func NewFeatureGate(m map[string]bool) (*featureGate, error) {
	known := make(map[Feature]bool)
	for k, v := range defaultFeatures {
		known[k] = v.Default
	}

	for k, v := range m {
		if !Supports(defaultFeatures, k) {
			return nil, fmt.Errorf("unrecognized feature-gate key: %s", k)
		}
		known[Feature(k)] = v
	}

	enabledValue := &atomic.Value{}
	enabledValue.Store(known)

	return &featureGate{
		enabled: enabledValue,
	}, nil
}

// Enabled indicates whether a feature name has been enabled
func (f *featureGate) Enabled(key Feature) bool {
	if v, ok := f.enabled.Load().(map[Feature]bool)[key]; ok {
		return v
	}

	panic(fmt.Errorf("feature %q is not registered in FeatureGate", key))
}

// Set feature gate for known features.
func (f *featureGate) Set(key Feature, value bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, known := defaultFeatures[key]; !known {
		return fmt.Errorf("unrecognized feature gate: %s", key)
	}

	enabled := map[Feature]bool{}
	for k, v := range f.enabled.Load().(map[Feature]bool) {
		enabled[k] = v
	}
	enabled[key] = value

	f.enabled.Store(enabled)

	return nil
}
