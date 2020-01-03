package net

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	GetLBIPTimeoutSec = 30
	GetLBIPPeriodSec  = 2
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

// GetNodeIP returns a node IP address
func GetNodeIP(cli client.Client) (string, error) {
	nodeLst := &v1.NodeList{}
	if err := cli.List(context.TODO(), nodeLst); err != nil {
		return "", err
	}
	if len(nodeLst.Items) == 0 {
		return "", errors.New("there is no available nodes")
	}
	for _, addr := range nodeLst.Items[0].Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", errors.New("there is no 'NodeInternalIP' type address")
}

// GetLBIP returns the external ip address assigned to Loadbalancer by
// cloud provider
func GetLBIP(name, namespace string, cli client.Client) (string, error) {
	timeout := time.After(GetLBIPTimeoutSec * time.Second)
	for {
		period := time.After(GetLBIPPeriodSec * time.Second)
		select {
		case <-timeout:
			return "", fmt.Errorf("Get LoadBalancer IP timeout for svc %s:%s",
				namespace, name)
		case <-period:
			// if external IP is not assigned to LB yet, we will
			// retry to get in period second
			svc := &v1.Service{}
			err := cli.Get(context.TODO(), types.NamespacedName{
				Namespace: namespace,
				Name:      name}, svc)
			if err != nil {
				return "", err
			}
			if len(svc.Status.LoadBalancer.Ingress) != 0 {
				return svc.Status.LoadBalancer.Ingress[0].IP, nil
			}
		}
	}
}
