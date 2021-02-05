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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type Patroller struct {
	// objectKind is the kind of target object this controller watched.
	objectKind string

	Options
}

// Options are the arguments for creating a new Patrol.
type Options struct {
	name       string
	Reconciler reconciler.PatrolReconciler
	Period     time.Duration
}

func NewPatroller(objectType runtime.Object, rc reconciler.PatrolReconciler, opts ...OptConfig) (*Patroller, error) {
	kinds, _, err := scheme.Scheme.ObjectKinds(objectType)
	if err != nil || len(kinds) == 0 {
		return nil, fmt.Errorf("patroller: unknown object kind %+v", objectType)
	}

	p := &Patroller{
		objectKind: kinds[0].Kind,
		Options: Options{
			name:       fmt.Sprintf("%s-patroller", strings.ToLower(kinds[0].Kind)),
			Reconciler: rc,
			Period:     60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(&p.Options)
	}

	if p.Reconciler == nil {
		return nil, fmt.Errorf("patroller %q: must specify patrol reconciler", p.objectKind)
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
