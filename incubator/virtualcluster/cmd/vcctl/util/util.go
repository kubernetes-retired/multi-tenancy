package util

import (
	"io/ioutil"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// yamlToObj read yaml from yamlPath and deserialize to a runtime.Object
// NOTE: make sure the target object type is added to scheme
func YamlToObj(scheme *runtime.Scheme, yamlPath string) (runtime.Object, error) {
	yamlFn, err := os.OpenFile(yamlPath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	yamlContent, err := ioutil.ReadAll(yamlFn)
	if err != nil {
		return nil, err
	}

	decode := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode
	obj, _, err := decode([]byte(yamlContent), nil, nil)
	if err != nil {
		return nil, err
	}

	return obj, nil
}
