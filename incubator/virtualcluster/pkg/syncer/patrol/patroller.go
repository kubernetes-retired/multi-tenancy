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

package patrol

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type Patroller struct {
	name string

	Options
}

// Options are the arguments for creating a new UpwardController.
type Options struct {
	Reconciler reconciler.PatrolReconciler
	Period     time.Duration
}

func NewPatroller(name string, options Options) (*Patroller, error) {
	if options.Reconciler == nil {
		return nil, fmt.Errorf("must specify patrol reconciler")
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("must specify Name for patrol reconciler")
	}
	p := &Patroller{
		name:    name,
		Options: options,
	}
	if p.Period == 0 {
		p.Period = 60 * time.Second
	}
	return p, nil
}

func (p *Patroller) Start(stop <-chan struct{}) {
	klog.Infof("start periodic checker %s", p.name)
	wait.Until(p.Reconciler.PatrollerDo, p.Period, stop)
}
