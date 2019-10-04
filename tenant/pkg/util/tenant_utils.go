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

package tenantnamespace

import (
	tenancyv1alpha1 "github.com/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
)

// GetTenantNamespaceName returns the tenant namespace name based on following conditions:
// If the tenant requires all tenant namespaces to have tenant admin namespace name as
// prefix (i.e., prefix is true), the namespace of the tenantnamespace instance is used
// to prefix the name in the spec. If the name in the spec is empty, the name of the
// tenantnamespace instance is used.
func GetTenantNamespaceName(prefix bool, instance *tenancyv1alpha1.TenantNamespace) string {
	name := instance.Spec.Name
	if name == "" {
		name = instance.Name
	}
	if prefix {
		name = instance.Namespace + "-" + name
	}
	return name
}
