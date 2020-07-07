package serviceutil

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type ServiceConfig struct {
	Type     v1.ServiceType
	Selector map[string]string
}

func (s *ServiceConfig) CreateServiceSpec() *v1.Service {
	serviceName := "service-" + string(uuid.NewUUID())
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: v1.ServiceSpec{
			Selector: s.Selector,
		},
	}
	service.Spec.Type = s.Type
	service.Spec.Ports = []v1.ServicePort{
		{Port: 80, Name: "http", Protocol: v1.ProtocolTCP},
	}
	return service
}
