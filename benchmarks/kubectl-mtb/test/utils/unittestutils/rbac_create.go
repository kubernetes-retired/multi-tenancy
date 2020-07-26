package unittestutils

import (
	"fmt"

	v1 "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (client *TestClient) CreateRole(roleName string, policy []v1.PolicyRule) (*v1.Role, error) {
	role := &v1.Role{
		TypeMeta: meta.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: meta.ObjectMeta{Name: roleName},
		Rules:      policy,
	}

	client.RoleName = roleName

	role, err := client.K8sClient.RbacV1().Roles(client.Namespace).Create(client.Context, role, meta.CreateOptions{})
	if err != nil {
		return nil, err
	} 

	return role, nil
}

func (client *TestClient) CreateRoleBinding(roleBindingName string, role *v1.Role) (*v1.RoleBinding, error) {
	subject := v1.Subject{
		Kind:      v1.ServiceAccountKind,
		APIGroup:  "",
		Name:      client.ServiceAccount.Name,
		Namespace: client.ServiceAccount.Namespace,
	}
	roleref := v1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     role.ObjectMeta.Name,
	}

	roleBinding := &v1.RoleBinding{
		TypeMeta: meta.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: meta.ObjectMeta{Name: roleBindingName},
		Subjects:   []v1.Subject{subject},
		RoleRef:    roleref,
	}

	client.RoleBindingName = roleBindingName

	_, err := client.K8sClient.RbacV1().RoleBindings(client.Namespace).Create(client.Context, roleBinding, meta.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (client *TestClient) DeleteRole() error {
	var gracePeriodSeconds int64 = 0
	deleteOptions := meta.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	// Delete Role binding
	if err := client.K8sClient.RbacV1().RoleBindings(client.Namespace).Delete(client.Context, client.RoleBindingName, deleteOptions); err != nil {
		fmt.Println(err.Error())
		return err
	}

	// Delete Role
	if err := client.K8sClient.RbacV1().Roles(client.Namespace).Delete(client.Context, client.RoleName, deleteOptions); err != nil {
		fmt.Println(err.Error())
		return err
	}

	return nil
}
