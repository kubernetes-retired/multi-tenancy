package metadata

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetLabel(inst metav1.Object, label string) (string, bool) {
	labels := inst.GetLabels()
	if labels == nil {
		return "", false
	}
	value, ok := labels[label]
	return value, ok
}

func SetLabel(inst metav1.Object, label string, value string) {
	labels := inst.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[label] = value
	inst.SetLabels(labels)
}

func SetAnnotation(inst metav1.Object, annotation string, value string) {
	annotations := inst.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotation] = value
	inst.SetAnnotations(annotations)
}
