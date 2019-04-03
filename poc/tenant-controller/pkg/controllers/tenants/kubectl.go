// Copyright 2017 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tenants

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8srt "k8s.io/apimachinery/pkg/runtime"
	json "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

// kubectlHelper invokes kubectl to manipulate Kubernetes resources.
// Because resource diff/merge logic is very complicated inside kubectl,
// it's best to leave that work to kubectl, instead of re-implement that
// in the controller.
type kubectlHelper struct {
	buf       bytes.Buffer
	namespace string
	args      []string
}

func newKubeCtl() *kubectlHelper {
	return &kubectlHelper{}
}

func (k *kubectlHelper) addObjects(objs ...k8srt.Object) *kubectlHelper {
	s := json.NewSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, true)
	for _, obj := range objs {
		copied := obj.DeepCopyObject()
		// clear namespace as it will be set on kubectl command line.
		copied.(metav1.Object).SetNamespace("")
		if err := s.Encode(copied, &k.buf); err != nil {
			panic(err)
		}
	}
	return k
}

func (k *kubectlHelper) withNamespace(ns string) *kubectlHelper {
	k.namespace = ns
	return k
}

func (k *kubectlHelper) withArgs(args ...string) *kubectlHelper {
	k.args = append(k.args, args...)
	return k
}

func (k *kubectlHelper) exec(command string, args ...string) error {
	cmd := exec.Command("kubectl")
	if k.namespace != "" {
		cmd.Args = append(cmd.Args, "-n", k.namespace)
	}
	cmd.Args = append(append(append(cmd.Args, k.args...), command), args...)
	cmd.Env = os.Environ()
	cmd.Stdin = &k.buf
	out, err := cmd.CombinedOutput()
	glog.V(4).Infoln("kubectl output:\n" + string(out))
	if err != nil {
		glog.Errorf("kubectl error: %v", err)
		return err
	}
	return nil
}

func (k *kubectlHelper) apply() error {
	return k.exec("apply", "-f", "-")
}

func (k *kubectlHelper) delete() error {
	return k.exec("delete", "-f", "-")
}
