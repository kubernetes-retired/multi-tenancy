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

package utils

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
)

func GetSecret(client v1core.CoreV1Interface, namespace, saName string) (*v1.Secret, error) {
	sa, err := client.ServiceAccounts(namespace).Get(saName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service account: %v", err)
	}
	if len(sa.Secrets) == 0 {
		return nil, fmt.Errorf("secret in serivce account is not ready")
	}

	return client.Secrets(namespace).Get(sa.Secrets[0].Name, metav1.GetOptions{})
}
