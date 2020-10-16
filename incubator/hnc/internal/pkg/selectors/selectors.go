// Package selectors contains exceptions related utilities, including check the validity of selector,
// treeSelector, and noneSelector, and parsing these three selectors.
package selectors

import (
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

func GetSelectorAnnotation(inst *unstructured.Unstructured) string {
	annot := inst.GetAnnotations()
	return annot[api.AnnotationSelector]
}

// GetTreeSelector is similar to a regular selector, except that it adds the LabelTreeDepthSuffix to every string
// To transform a tree selector into a regular label selector, we follow these steps:
// 1. get the treeSelector annotation if it exists
// 2. convert the annotation string to a slice of strings seperated by comma, because user is allowed to put multiple selectors
// 3. append the LabelTreeDepthSuffix to each of the treeSelector string
// 4. combine them into a single string connected by comma
func GetTreeSelector(inst *unstructured.Unstructured) (labels.Selector, error) {
	annot := inst.GetAnnotations()
	treeSelectorStr, ok := annot[api.AnnotationTreeSelector]
	if !ok {
		return nil, nil
	}
	strs := strings.Split(treeSelectorStr, ",")
	selectorStr := ""
	for i, str := range strs {
		selectorStr = selectorStr + str + api.LabelTreeDepthSuffix
		if i < len(strs)-1 {
			selectorStr = selectorStr + ", "
		}
	}
	treeSelector, err := getSelectorFromString(selectorStr)
	if err != nil {
		return nil, fmt.Errorf("while parsing %q: %w", api.AnnotationTreeSelector, err)
	}
	return treeSelector, nil
}

// GetSelector returns the selector on a given object if it exists
func GetSelector(inst *unstructured.Unstructured) (labels.Selector, error) {
	selector, err := getSelectorFromString(GetSelectorAnnotation(inst))
	if err != nil {
		return nil, fmt.Errorf("while parsing %q: %w", api.AnnotationSelector, err)
	}
	return selector, nil
}

// getSelectorFromString converts the given string to a selector
// Note: any invalid Selector value will cause this object not propagating to any child namespace
func getSelectorFromString(str string) (labels.Selector, error) {
	labelSelector, err := v1.ParseToLabelSelector(str)
	if err != nil {
		return nil, err
	}
	selector, err := v1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, err
	}
	return selector, nil
}
