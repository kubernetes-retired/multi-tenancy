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

package mccontroller

import (
	v1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjCRF = func(objectType runtime.Object) runtime.Object

var TargetObjCR [](ObjCRF)
var TargetObjCRList [](ObjCRF)

func AddTargetObjCR(objCRF ObjCRF) {
	TargetObjCR = append(TargetObjCR, objCRF)
}

func AddTargetObjCRList(objCRF ObjCRF) {
	TargetObjCRList = append(TargetObjCRList, objCRF)
}

func getTargetObject(objectType runtime.Object) runtime.Object {
	for _, f := range TargetObjCR {
		ro := f(objectType)
		if ro != nil {
			return ro
		}
	}
	return nil
}

func getTargetObjectList(objectType runtime.Object) runtime.Object {
	for _, f := range TargetObjCRList {
		ro := f(objectType)
		if ro != nil {
			return ro
		}
	}
	return nil
}

func init() {
	AddTargetObjCR(getTargetObj)
	AddTargetObjCRList(getTargetObjList)
}

func getTargetObj(objectType runtime.Object) runtime.Object {
	switch objectType.(type) {
	case *v1.ConfigMap:
		return &v1.ConfigMap{}
	case *v1.Namespace:
		return &v1.Namespace{}
	case *v1.Node:
		return &v1.Node{}
	case *v1.Event:
		return &v1.Event{}
	case *v1.Pod:
		return &v1.Pod{}
	case *v1.Secret:
		return &v1.Secret{}
	case *v1.Service:
		return &v1.Service{}
	case *v1.ServiceAccount:
		return &v1.ServiceAccount{}
	case *storagev1.StorageClass:
		return &storagev1.StorageClass{}
	case *v1.PersistentVolumeClaim:
		return &v1.PersistentVolumeClaim{}
	case *v1.PersistentVolume:
		return &v1.PersistentVolume{}
	case *v1.Endpoints:
		return &v1.Endpoints{}
	case *schedulingv1.PriorityClass:
		return &schedulingv1.PriorityClass{}
	case *extensionsv1beta1.Ingress:
		return &extensionsv1beta1.Ingress{}
	default:
		return nil
	}
}

func getTargetObjList(objectType runtime.Object) runtime.Object {
	switch objectType.(type) {
	case *v1.ConfigMap:
		return &v1.ConfigMapList{}
	case *v1.Namespace:
		return &v1.NamespaceList{}
	case *v1.Node:
		return &v1.NodeList{}
	case *v1.Event:
		return &v1.EventList{}
	case *v1.Pod:
		return &v1.PodList{}
	case *v1.Secret:
		return &v1.SecretList{}
	case *v1.Service:
		return &v1.ServiceList{}
	case *v1.ServiceAccount:
		return &v1.ServiceAccountList{}
	case *storagev1.StorageClass:
		return &storagev1.StorageClassList{}
	case *v1.PersistentVolumeClaim:
		return &v1.PersistentVolumeClaimList{}
	case *v1.PersistentVolume:
		return &v1.PersistentVolumeList{}
	case *v1.Endpoints:
		return &v1.EndpointsList{}
	case *schedulingv1.PriorityClass:
		return &schedulingv1.PriorityClassList{}
	case *extensionsv1beta1.Ingress:
		return &extensionsv1beta1.IngressList{}
	default:
		return nil
	}
}
