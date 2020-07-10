package unittestutils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

func (self *TestClient) CreatePolicy(resourcePath string) error {
	var data []byte
	if strings.Contains(resourcePath, "yaml") {
		data, err = loadFile(resourcePath)
	} else {
		data = []byte(resourcePath)
	}
	if err != nil {
		return fmt.Errorf("could not load file")
	}

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	pBytes := bytes.Split(data, []byte("---"))
	for _, policy := range pBytes {

		// Prepare a RESTMapper to find GVR
		dc, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			fmt.Println(err.Error())
		}

		mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

		// Prepare the dynamic unittestutils
		dyn, err := dynamic.NewForConfig(config)
		if err != nil {
			fmt.Println(err.Error())
		}

		// Decode YAML manifest into unstructured.Unstructured
		obj := &unstructured.Unstructured{}

		_, gvk, err := decUnstructured.Decode([]byte(policy), nil, obj)

		if err != nil {
			fmt.Println(err.Error())
		}
		// Find GVR
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			fmt.Println(err.Error())
		}

		// Obtain REST interface for the GVR
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespaced resources should specify the namespace
			self.DynamicResource = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			// for cluster-wide resources
			self.DynamicResource = dyn.Resource(mapping.Resource)
		}

		// Create or Update the object with SSA
		// types.ApplyPatchType indicates SSA.
		// FieldManager specifies the field owner ID.
		_, err = self.DynamicResource.Create(self.Context, obj, metav1.CreateOptions{})
	}
	return err
}

func (self *TestClient) DeletePolicy() error {
	err = self.DynamicResource.Delete(self.Context, self.PolicyName, metav1.DeleteOptions{})
	return err
}

func loadFile(path string) ([]byte, error) {
	fmt.Println("Reading", path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return ioutil.ReadFile(path)
}
