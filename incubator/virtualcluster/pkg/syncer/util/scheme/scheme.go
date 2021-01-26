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

package scheme

import (
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
)

var Scheme = NewScheme()

// scheme record the mapping from runtime object type to their list type.
// schemes are not expected to change at runtime and are only threadsafe after
// registration is complete.
type scheme struct {
	mu               sync.Mutex
	typeToTypeObject map[reflect.Type]runtime.Object
	typeToListObject map[reflect.Type]runtime.Object
}

func NewScheme() *scheme {
	return &scheme{
		typeToTypeObject: make(map[reflect.Type]runtime.Object),
		typeToListObject: make(map[reflect.Type]runtime.Object),
	}
}

// AddKnownTypePair register object type and object list type pair.
func (s *scheme) AddKnownTypePair(objAndObjLists ...runtime.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(objAndObjLists)%2 != 0 {
		panic("object and object list should be in pair")
	}

	for i := 0; i < len(objAndObjLists); i += 2 {
		t := reflect.TypeOf(objAndObjLists[i])
		listT := reflect.TypeOf(objAndObjLists[i+1])

		s.typeToTypeObject[t] = reflect.New(t.Elem()).Interface().(runtime.Object)
		s.typeToListObject[t] = reflect.New(listT.Elem()).Interface().(runtime.Object)
	}
}

// NewObjectList deep copy from zero list type value.
// which is faster than directly reflect.New in runtime.
func (s *scheme) NewObjectList(obj runtime.Object) runtime.Object {
	t := reflect.TypeOf(obj)

	listT, ok := s.typeToListObject[t]
	if !ok {
		return nil
	}

	return listT.DeepCopyObject()
}

// NewObject deep copy from zero type value.
// which is faster than directly reflect.New in runtime.
func (s *scheme) NewObject(obj runtime.Object) runtime.Object {
	t := reflect.TypeOf(obj)

	to, ok := s.typeToTypeObject[t]
	if !ok {
		return nil
	}

	return to.DeepCopyObject()
}
