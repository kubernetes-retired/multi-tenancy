package podutil

import (
	"fmt"

	"github.com/creasty/defaults"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// PodSpec is a struct containing all arguments for creating a pod.
type PodSpec struct {
	NS                       string                      `default:""`
	Pvclaims                 []*v1.PersistentVolumeClaim `default:"-"`
	InlineVolumeSources      []*v1.VolumeSource          `default:"-"`
	HostNetwork              bool                        `default:"-"`
	Command                  string                      `default:""`
	HostIPC                  bool                        `default:"false"`
	HostPID                  bool                        `default:"false"`
	seLinuxLabel             *v1.SELinuxOptions
	fsGroup                  *int64
	RunAsNonRoot             bool               `default:"-"`
	IsPrivileged             bool               `default:"false"`
	Capability               []v1.Capability    `default:"-"`
	Ports                    []v1.ContainerPort `default:"-"`
	AllowPrivilegeEscalation bool               `default:"-"`
}

// SetDefaults usage := https://github.com/creasty/defaults#usage
func (p *PodSpec) SetDefaults() error {
	if err := defaults.Set(p); err != nil {
		return fmt.Errorf("it should not return an error: %v", err)
	}
	return nil
}

// MakeSecPod returns a pod definition based on the namespace. The pod references the PVC's name. A slice of BASH commands can be supplied as args to be run by the pod.
func (p PodSpec) MakeSecPod() *v1.Pod {
	if len(p.Command) == 0 {
		p.Command = "trap exit TERM; while true; do sleep 1; done"
	}
	podName := "security-context-" + string(uuid.NewUUID())
	if p.fsGroup == nil {
		p.fsGroup = func(i int64) *int64 {
			return &i
		}(1000)
	}
	podSpec := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: p.NS,
		},
		Spec: v1.PodSpec{
			HostNetwork: p.HostNetwork,
			HostIPC:     p.HostIPC,
			HostPID:     p.HostPID,
			SecurityContext: &v1.PodSecurityContext{
				FSGroup:      p.fsGroup,
				RunAsNonRoot: &p.RunAsNonRoot,
			},
			Containers: []v1.Container{
				{
					Name:  "write-pod",
					Image: imageutils.GetE2EImage(imageutils.BusyBox),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"cpu":    resource.MustParse("0m"),
							"memory": resource.MustParse("0Gi"),
						},
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("0m"),
							"memory": resource.MustParse("0Gi"),
						},
					},
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", p.Command},
					Ports:   p.Ports,
					SecurityContext: &v1.SecurityContext{
						RunAsNonRoot: &p.RunAsNonRoot,
						Privileged:   &p.IsPrivileged,
						Capabilities: &v1.Capabilities{
							Add: p.Capability,
						},
						AllowPrivilegeEscalation: &p.AllowPrivilegeEscalation,
					},
				},
			},
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	var volumeMounts = make([]v1.VolumeMount, 0)
	var volumeDevices = make([]v1.VolumeDevice, 0)
	var volumes = make([]v1.Volume, len(p.Pvclaims)+len(p.InlineVolumeSources))
	volumeIndex := 0
	for _, pvclaim := range p.Pvclaims {
		volumename := fmt.Sprintf("volume%v", volumeIndex+1)
		if pvclaim.Spec.VolumeMode != nil && *pvclaim.Spec.VolumeMode == v1.PersistentVolumeBlock {
			volumeDevices = append(volumeDevices, v1.VolumeDevice{Name: volumename, DevicePath: "/mnt/" + volumename})
		} else {
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: volumename, MountPath: "/mnt/" + volumename})
		}

		volumes[volumeIndex] = v1.Volume{Name: volumename, VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: pvclaim.Name, ReadOnly: false}}}
		volumeIndex++
	}
	for _, src := range p.InlineVolumeSources {
		volumename := fmt.Sprintf("volume%v", volumeIndex+1)
		// In-line volumes can be only filesystem, not block.
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: volumename, MountPath: "/mnt/" + volumename})
		volumes[volumeIndex] = v1.Volume{Name: volumename, VolumeSource: *src}
		volumeIndex++
	}

	podSpec.Spec.Containers[0].VolumeMounts = volumeMounts
	podSpec.Spec.Containers[0].VolumeDevices = volumeDevices
	podSpec.Spec.Volumes = volumes
	podSpec.Spec.SecurityContext.SELinuxOptions = p.seLinuxLabel
	return podSpec
}
