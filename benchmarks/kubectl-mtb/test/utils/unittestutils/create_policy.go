package unittestutils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"net/http"
	"os"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

func (self *TestClient) CreatePolicy(resourcePath string) error {

	var data []byte
	if utils.IsValidUrl(resourcePath) {
		data, err = readYAMLFromUrl(resourcePath)
	} else {
		data, err = loadFile(resourcePath)
	}
	if err != nil {
		return nil
	}

	pBytes := bytes.Split(data, []byte("---"))
	for _, policy := range pBytes {
		// Prepare a RESTMapper to find GVR
		dc, err := discovery.NewDiscoveryClientForConfig(self.Config)
		if err != nil {
			fmt.Println(err.Error())
		}
		mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

		// Prepare the dynamic unittestutils
		dyn, err := dynamic.NewForConfig(self.Config)
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

func readYAMLFromUrl(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	respByte := buf.Bytes()
	return respByte, nil
}

func loadFile(path string) ([]byte, error) {
	fmt.Println("Reading", path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return ioutil.ReadFile(path)
}
