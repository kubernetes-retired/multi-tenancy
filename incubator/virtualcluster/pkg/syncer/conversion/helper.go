/*
Copyright 2019 The Kubernetes Authors.

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

package conversion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
	listersv1 "k8s.io/client-go/listers/core/v1"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
)

const (
	masterServiceNamespace = metav1.NamespaceDefault
)

var masterServices = sets.NewString("kubernetes")

// ToClusterKey make a unique id for a virtual cluster object.
// The key uses the format <namespace>-<name> unless <namespace> is empty, then
// it's just <name>.
func ToClusterKey(vc *v1alpha1.Virtualcluster) string {
	if len(vc.GetNamespace()) > 0 {
		return vc.GetNamespace() + "-" + vc.GetName()
	}
	return vc.GetName()
}

func ToSuperMasterNamespace(cluster, ns string) string {
	targetNamespace := strings.Join([]string{cluster, ns}, "-")
	if len(targetNamespace) > validation.DNS1123SubdomainMaxLength {
		digest := sha256.Sum256([]byte(targetNamespace))
		return targetNamespace[0:57] + "-" + hex.EncodeToString(digest[0:])[0:5]
	}
	return targetNamespace
}

func GetVirtualNamespace(nsLister listersv1.NamespaceLister, pNamespace string) (cluster, namespace string, err error) {
	vcInfo, err := nsLister.Get(pNamespace)
	if err != nil {
		return
	}

	if v, ok := vcInfo.GetAnnotations()[constants.LabelCluster]; ok {
		cluster = v
	}
	if v, ok := vcInfo.GetAnnotations()[constants.LabelNamespace]; ok {
		namespace = v
	}
	return
}

func GetVirtualOwner(obj runtime.Object) (cluster, namespace string) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return "", ""
	}

	cluster = meta.GetAnnotations()[constants.LabelCluster]
	namespace = strings.TrimPrefix(meta.GetNamespace(), cluster+"-")
	return cluster, namespace
}

func BuildMetadata(cluster, targetNamespace string, obj runtime.Object) (runtime.Object, error) {
	target := obj.DeepCopyObject()
	m, err := meta.Accessor(target)
	if err != nil {
		return nil, err
	}

	ownerReferencesStr, err := json.Marshal(m.GetOwnerReferences())
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal owner references")
	}

	var tenantScopeMeta = map[string]string{
		constants.LabelCluster:         cluster,
		constants.LabelUID:             string(m.GetUID()),
		constants.LabelOwnerReferences: string(ownerReferencesStr),
		constants.LabelNamespace:       m.GetNamespace(),
	}

	ResetMetadata(m)
	if len(targetNamespace) > 0 {
		m.SetNamespace(targetNamespace)
	}

	anno := m.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	for k, v := range tenantScopeMeta {
		anno[k] = v
	}
	m.SetAnnotations(anno)

	return target, nil
}

func BuildSuperMasterNamespace(cluster string, obj runtime.Object) (runtime.Object, error) {
	target := obj.DeepCopyObject()
	m, err := meta.Accessor(target)
	if err != nil {
		return nil, err
	}

	anno := m.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	anno[constants.LabelCluster] = cluster
	anno[constants.LabelUID] = string(m.GetUID())
	anno[constants.LabelNamespace] = m.GetName()
	m.SetAnnotations(anno)

	ResetMetadata(m)

	targetName := strings.Join([]string{cluster, m.GetName()}, "-")
	m.SetName(targetName)
	return target, nil
}

func ResetMetadata(obj metav1.Object) {
	obj.SetSelfLink("")
	obj.SetUID("")
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetDeletionTimestamp(nil)
	obj.SetDeletionGracePeriodSeconds(nil)
	obj.SetOwnerReferences(nil)
	obj.SetFinalizers(nil)
	obj.SetClusterName("")
	obj.SetInitializers(nil)
}

func BuildVirtualPodEvent(cluster string, pEvent *v1.Event, vPod *v1.Pod) *v1.Event {
	vEvent := pEvent.DeepCopy()
	ResetMetadata(vEvent)
	vEvent.SetNamespace(vPod.Namespace)
	vEvent.InvolvedObject.Namespace = vPod.Namespace
	vEvent.InvolvedObject.UID = vPod.UID
	vEvent.InvolvedObject.ResourceVersion = ""

	vEvent.Message = strings.ReplaceAll(vEvent.Message, cluster+"-", "")
	vEvent.Message = strings.ReplaceAll(vEvent.Message, cluster, "")

	return vEvent
}

func BuildVirtualStorageClass(cluster string, pStorageClass *storagev1.StorageClass) *storagev1.StorageClass {
	vStorageClass := pStorageClass.DeepCopy()
	ResetMetadata(vStorageClass)
	return vStorageClass
}

func BuildKubeConfigSecret(clusterName string, vPod *v1.Pod, kubeConfig []byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("vc-kubeconfig-%s", string(uuid.NewUUID())),
			Annotations: map[string]string{
				constants.LabelCluster:   clusterName,
				constants.LabelNamespace: vPod.Namespace,
				// record the owner pod. checker should do gc for us.
				constants.LabelOwnerReferences: fmt.Sprintf(`'[{"apiVersion":"core/v1","kind":"Pod","name":"%s","uid":"%s"}]'`, vPod.Name, vPod.UID),
			},
		},
		Data: map[string][]byte{
			"kubeconfig": kubeConfig,
		},
		Type: v1.SecretTypeOpaque,
	}
}
