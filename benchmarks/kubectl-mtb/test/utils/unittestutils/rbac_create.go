package unittestutils

import (
	"fmt"

	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (self *TestClient) CreateRole(roleName string, policy []v1.PolicyRule) (*v1.Role, error) {
	role := &v1.Role{
		TypeMeta: meta.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: meta.ObjectMeta{Name: roleName},
		Rules:      policy,
	}

	self.RoleName = roleName

	if role, err := self.Kubernetes.RbacV1().Roles(self.Namespace).Create(self.Context, role, meta.CreateOptions{}); err == nil {
		return role, err
	} else if errors.IsAlreadyExists(err) {
		fmt.Println(err.Error())
		return self.Kubernetes.RbacV1().Roles(self.Namespace).Get(self.Context, roleName, meta.GetOptions{})
	} else {
		return nil, err
	}
}

func (self *TestClient) CreateRoleBinding(roleBindingName string, role *v1.Role) (*v1.RoleBinding, error) {
	subject := v1.Subject{
		Kind:      v1.ServiceAccountKind,
		APIGroup:  "",
		Name:      self.ServiceAccount,
		Namespace: "default",
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

	self.RoleBindingName = roleBindingName

	if roleBinding, err := self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Create(self.Context, roleBinding, meta.CreateOptions{}); err == nil {
		return roleBinding, nil
	} else if errors.IsAlreadyExists(err) {
		fmt.Println(err.Error())
		return self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Get(self.Context, self.RoleBindingName, meta.GetOptions{})
	} else {
		return nil, err
	}
}

func (self *TestClient) DeleteRole() error {
	var gracePeriodSeconds int64 = 0
	deleteOptions := meta.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	// Delete Role binding
	if err := self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Delete(self.Context, self.RoleBindingName, deleteOptions); err != nil {
		fmt.Println(err.Error())
	}

	// Delete Role
	if err := self.Kubernetes.RbacV1().Roles(self.Namespace).Delete(self.Context, self.RoleName, deleteOptions); err != nil {
		fmt.Println(err.Error())
	}

	return err
}
