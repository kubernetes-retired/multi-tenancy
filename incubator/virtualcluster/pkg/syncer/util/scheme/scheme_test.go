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
	"math/rand"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
)

func TestScheme_NewObject(t *testing.T) {
	s := NewScheme()

	s.AddKnownTypePair(&v1.Pod{}, &v1.PodList{})

	newT := s.NewObject(&v1.Pod{})

	kinds, _, err := k8sscheme.Scheme.ObjectKinds(newT)
	if err != nil || len(kinds) == 0 {
		t.Errorf("expected known pod type: %v", err)
	}

	if kinds[0].Kind != "Pod" {
		t.Errorf("expected pod type, got %v", kinds[0].Kind)
	}

	newT = s.NewObject(&v1.ConfigMap{})
	if newT != nil {
		t.Errorf("got non nil object for unkown type")
	}

	s.AddKnownTypePair(&v1.Secret{}, &v1.SecretList{}, &v1.ConfigMap{}, &v1.ConfigMapList{})
	newT = s.NewObject(&v1.ConfigMap{})

	kinds, _, err = k8sscheme.Scheme.ObjectKinds(newT)
	if err != nil || len(kinds) == 0 {
		t.Errorf("expected known configmap type after multi pair register: %v", err)
	}

	if kinds[0].Kind != "ConfigMap" {
		t.Errorf("expected configmap type after multi pair register, got %v", kinds[0].Kind)
	}
}

func TestScheme_NewObjectList(t *testing.T) {
	s := NewScheme()

	s.AddKnownTypePair(&v1.Pod{}, &v1.PodList{})

	newList := s.NewObjectList(&v1.Pod{})

	kinds, _, err := k8sscheme.Scheme.ObjectKinds(newList)
	if err != nil || len(kinds) == 0 {
		t.Errorf("expected known podList type: %v", err)
	}

	if kinds[0].Kind != "PodList" {
		t.Errorf("expected podList type, got %v", kinds[0].Kind)
	}

	newList = s.NewObjectList(&v1.ConfigMap{})
	if newList != nil {
		t.Errorf("got non nil object for unkown type")
	}
}

func Benchmark_NewObject(b *testing.B) {
	b.ReportAllocs()
	rand.Seed(time.Now().UnixNano())

	s := NewScheme()
	s.AddKnownTypePair(&v1.Pod{}, &v1.PodList{})

	b.ResetTimer()
	for i := 0; i < 1000; i++ {
		s.NewObject(&v1.Pod{})
	}
}

func Benchmark_NewObjectList(b *testing.B) {
	b.ReportAllocs()
	rand.Seed(time.Now().UnixNano())

	s := NewScheme()
	s.AddKnownTypePair(&v1.Pod{}, &v1.PodList{})

	b.ResetTimer()
	for i := 0; i < 1000; i++ {
		s.NewObjectList(&v1.Pod{})
	}
}
