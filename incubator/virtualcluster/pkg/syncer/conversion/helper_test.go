package conversion

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

func Test_mutateDownwardAPIField(t *testing.T) {
	aPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "ns",
			UID:       types.UID("5033b5b7-104f-11ea-b309-525400c042d5"),
		},
	}

	for _, tt := range []struct {
		name        string
		pod         *v1.Pod
		env         *v1.EnvVar
		expectedEnv *v1.EnvVar
	}{
		{
			name: "env without fieldRef",
			pod:  aPod,
			env: &v1.EnvVar{
				Name:      "env_name",
				Value:     "env_value",
				ValueFrom: nil,
			},
			expectedEnv: &v1.EnvVar{
				Name:      "env_name",
				Value:     "env_value",
				ValueFrom: nil,
			},
		},
		{
			name: "env with other fieldRef",
			pod:  aPod,
			env: &v1.EnvVar{
				Name: "env_name",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "spec.nodeName",
					},
				},
			},
			expectedEnv: &v1.EnvVar{
				Name: "env_name",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "spec.nodeName",
					},
				},
			},
		},
		{
			name: "env with metadata.name",
			pod:  aPod,
			env: &v1.EnvVar{
				Name: "env_name",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
			expectedEnv: &v1.EnvVar{
				Name:  "env_name",
				Value: aPod.Name,
			},
		},
		{
			name: "env with metadata.namespace",
			pod:  aPod,
			env: &v1.EnvVar{
				Name: "env_name",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.namespace",
					},
				},
			},
			expectedEnv: &v1.EnvVar{
				Name:  "env_name",
				Value: aPod.Namespace,
			},
		},
		{
			name: "env with metadata.uid",
			pod:  aPod,
			env: &v1.EnvVar{
				Name: "env_name",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.uid",
					},
				},
			},
			expectedEnv: &v1.EnvVar{
				Name:  "env_name",
				Value: string(aPod.UID),
			},
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			mutateDownwardAPIField(tt.env, tt.pod)
			if !equality.Semantic.DeepEqual(tt.env, tt.expectedEnv) {
				tc.Errorf("expected env %+v, got %+v", tt.expectedEnv, tt.env)
			}
		})
	}
}

func Test_mutateContainerSecret(t *testing.T) {
	saSecretMap := map[string]string{
		"service-token-secret-tenant": "service-token-secret",
	}
	for _, tt := range []struct {
		name              string
		container         *v1.Container
		saSecretMap       map[string]string
		vPod              *v1.Pod
		expectedContainer *v1.Container
	}{
		{
			name: "normal case",
			container: &v1.Container{
				VolumeMounts: []v1.VolumeMount{
					{
						Name:      "some-mount",
						MountPath: "/path/to/mount",
						ReadOnly:  true,
					},
					{
						Name:      "service-token-secret-tenant",
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
						ReadOnly:  true,
					},
				},
			},
			saSecretMap: saSecretMap,
			vPod: &v1.Pod{
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: "service-token-secret-tenant",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "service-token-secret-tenant",
								},
							},
						},
					},
				},
			},
			expectedContainer: &v1.Container{
				VolumeMounts: []v1.VolumeMount{
					{
						Name:      "some-mount",
						MountPath: "/path/to/mount",
						ReadOnly:  true,
					},
					{
						Name:      "service-token-secret",
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
						ReadOnly:  true,
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			mutateContainerSecret(tt.container, tt.saSecretMap, tt.vPod)
			if !equality.Semantic.DeepEqual(tt.container, tt.expectedContainer) {
				tc.Errorf("expected container %+v, got %+v", tt.expectedContainer, tt.container)
			}
		})
	}
}

func TestToClusterKey(t *testing.T) {
	for _, tt := range []struct {
		name        string
		vc          *v1alpha1.VirtualCluster
		expectedKey string
	}{
		{
			name: "normal vc",
			vc: &v1alpha1.VirtualCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "ns",
					UID:       "d64ea0c0-91f8-46f5-8643-c0cab32ab0cd",
				},
			},
			expectedKey: "ns-fd1b34-name",
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			key := ToClusterKey(tt.vc)
			if key != tt.expectedKey {
				tc.Errorf("expected key %s, got %s", tt.expectedKey, key)
			}
		})
	}
}
