package deploymentutil

import (
	"fmt"
	"github.com/creasty/defaults"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentSpec struct {
	DeploymentName string
	Replicas       int32
	PodLabels      map[string]string
	ImageName      string
	Image          string
	StrategyType   appsv1.DeploymentStrategyType
}

// SetDefaults usage := https://github.com/creasty/defaults#usage
func (d *DeploymentSpec) SetDefaults() error {
	if err := defaults.Set(d); err != nil {
		return fmt.Errorf("it should not return an error: %v", err)
	}
	return nil
}


func (d DeploymentSpec) GetDeployment() *appsv1.Deployment {
	zero := int64(0)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   d.DeploymentName,
			Labels: d.PodLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &d.Replicas,
			Selector: &metav1.LabelSelector{MatchLabels: d.PodLabels},
			Strategy: appsv1.DeploymentStrategy{
				Type: d.StrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: d.PodLabels,
				},
				Spec: v1.PodSpec{
					TerminationGracePeriodSeconds: &zero,
					Containers: []v1.Container{
						{
							Name:            d.ImageName,
							Image:           d.Image,
							SecurityContext: &v1.SecurityContext{},
							ImagePullPolicy: v1.PullAlways,
						},
					},
				},
			},
		},
	}
}
