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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type Patroller struct {
	name string
	// objectKind is the kind of target object this controller watched.
	objectKind string

	Options
}

// Options are the arguments for creating a new Patrol.
type Options struct {
	Reconciler reconciler.PatrolReconciler
	Period     time.Duration
}

func NewPatroller(name string, objectType runtime.Object, options Options) (*Patroller, error) {
	if options.Reconciler == nil {
		return nil, fmt.Errorf("must specify patrol reconciler")
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("must specify Name for patrol reconciler")
	}

	kinds, _, err := scheme.Scheme.ObjectKinds(objectType)
	if err != nil || len(kinds) == 0 {
		return nil, fmt.Errorf("unknown object kind %+v", objectType)
	}

	p := &Patroller{
		name:       name,
		objectKind: kinds[0].Kind,
		Options:    options,
	}
	if p.Period == 0 {
		p.Period = 60 * time.Second
	}
	return p, nil
}

func (p *Patroller) Start(stop <-chan struct{}) {
	klog.Infof("start periodic checker %s", p.name)
	wait.Until(p.run, p.Period, stop)
}

func (p *Patroller) run() {
	defer metrics.RecordCheckerScanDuration(p.objectKind, time.Now())
	p.Reconciler.PatrollerDo()
}
