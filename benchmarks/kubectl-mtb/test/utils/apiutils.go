package utils

import (
	"context"
	"fmt"
	"strings"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type GroupResource struct {
	APIGroup     string
	APIResource  metav1.APIResource
	ResourceName string
}

// RunAccessCheck checks that given client can perform the given verb on the resource or not
func RunAccessCheck(client *kubernetes.Clientset, namespace string, resource GroupResource, verb string) (bool, string, error) {
	var sar *authorizationv1.SelfSubjectAccessReview

	// Todo for non resource url
	sar = &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   namespace,
				Verb:        verb,
				Group:       resource.APIGroup,
				Resource:    resource.APIResource.Name,
				Subresource: "",
				Name:        resource.ResourceName,
			},
		},
	}

	response, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(context.TODO(), sar, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	if err != nil {
		return false, "", err
	}

	if response.Status.Allowed {
		return true, fmt.Sprintf("User can %s %s", verb, resource.APIResource.Name), nil
	}

	return false, fmt.Sprintf("User cannot %s %s", verb, resource.APIResource.Name), nil
}

func GetTenantResoureQuotas(tenantNamespace string, tclient *kubernetes.Clientset) []string {
	var tmpList string
	var tenantResourceQuotas []string
	resourcequotaList, err := tclient.CoreV1().ResourceQuotas(tenantNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
	}

	for _, resourcequota := range resourcequotaList.Items {
		for name := range resourcequota.Spec.Hard {
			if strings.Contains(tmpList, name.String()) {
				continue
			}

			tenantResourceQuotas = append(tenantResourceQuotas, name.String())
			tmpList = tmpList + name.String()
		}
	}

	return tenantResourceQuotas
}
