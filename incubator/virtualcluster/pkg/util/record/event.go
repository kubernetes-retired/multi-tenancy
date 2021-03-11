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

package record

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	clientgorecord "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/record/util"
)

type EventSinker struct {
	correlator *clientgorecord.EventCorrelator
}

var EventSinkerInstance = NewEventSinker(clientgorecord.CorrelatorOptions{})

func NewEventSinker(options clientgorecord.CorrelatorOptions) *EventSinker {
	return &EventSinker{correlator: clientgorecord.NewEventCorrelatorWithOptions(options)}
}

func (e *EventSinker) RecordToSink(sink clientgorecord.EventSink, event *v1.Event) error {
	var newEvent *v1.Event
	var err error

	result, err := e.correlator.EventCorrelate(event)
	if err != nil {
		return err
	}
	if result.Skip {
		return nil
	}

	updateExisting := result.Event.Count > 1
	if updateExisting {
		newEvent, err = sink.Patch(result.Event, result.Patch)
	}
	// Update can fail because the event may have been removed and it no longer exists.
	if !updateExisting || (updateExisting && util.IsKeyNotFoundError(err)) {
		// Making sure that ResourceVersion is empty on creation
		event.ResourceVersion = ""
		newEvent, err = sink.Create(result.Event)
	}
	if err == nil {
		// we need to update our event correlator with the server returned state to handle name/resourceversion
		e.correlator.UpdateState(newEvent)
		return nil
	}

	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
