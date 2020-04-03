package util

import (
	"io/ioutil"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// yamlToObj read yaml from yamlPath and deserialize to a runtime.Object
// NOTE: make sure the target object type is added to scheme
func YamlToObj(scheme *runtime.Scheme, yamlPath string) (runtime.Object, error) {
	yamlContent, err := getYamlContent(yamlPath)
	if err != nil {
		return nil, err
	}
	decode := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode
	obj, _, err := decode(yamlContent, nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// getYamlContent reads the yaml content from the `yamlPath`
func getYamlContent(yamlPath string) ([]byte, error) {
	if isURL(yamlPath) {
		// read from an URL
		resp, err := http.Get(yamlPath)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		yamlContent, err := ioutil.ReadAll(resp.Body)
		return yamlContent, nil
	}
	// read from a file
	yamlContent, err := ioutil.ReadFile(yamlPath)
	return yamlContent, err
}

// isURL checks if `path` is an URL
func isURL(path string) bool {
	return strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://")
}
