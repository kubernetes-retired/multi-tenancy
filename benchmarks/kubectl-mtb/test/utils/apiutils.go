package util

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type GroupResource struct {
	APIGroup    string
	APIResource metav1.APIResource
}

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
				Name:        "",
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