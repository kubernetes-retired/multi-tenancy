package net

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSvcNodePort returns the nodePort of service with given name
// in given namespace
func GetSvcNodePort(name, namespace string, cli client.Client) (int32, error) {
	svc := &v1.Service{}
	err := cli.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name}, svc)
	if err != nil {
		return 0, err
	}
	return svc.Spec.Ports[0].NodePort, nil
}
