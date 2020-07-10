package unittestutils

import (
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
		ObjectMeta: meta.ObjectMeta{Name: "role1"},
		Rules:      policy,
	}

	if role, err := self.Kubernetes.RbacV1().Roles(self.Namespace).Create(self.Context, role, meta.CreateOptions{}); err == nil {
		return role, err
	} else if errors.IsAlreadyExists(err) {
		self.Log.Infof("%s", err.Error())
		return self.Kubernetes.RbacV1().Roles(self.Namespace).Get(self.Context, "role1", meta.GetOptions{})
	} else {
		return nil, err
	}
}

func (self *TestClient) CreateRoleBinding(role *v1.Role) (*v1.RoleBinding, error) {

	subject := v1.Subject{
		Kind:      v1.ServiceAccountKind,
		APIGroup:  "",
		Name:      self.ServiceAccount,
		Namespace: "default",
	}
	roleref := v1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "role1",
	}

	roleBinding := &v1.RoleBinding{
		TypeMeta: meta.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: meta.ObjectMeta{Name: "role1"},
		Subjects:   []v1.Subject{subject},
		RoleRef:    roleref,
	}

	if roleBinding, err := self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Create(self.Context, roleBinding, meta.CreateOptions{}); err == nil {
		return roleBinding, nil
	} else if errors.IsAlreadyExists(err) {
		self.Log.Infof("%s", err.Error())
		return self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Get(self.Context, "role1", meta.GetOptions{})
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
	if err := self.Kubernetes.RbacV1().RoleBindings(self.Namespace).Delete(self.Context, self.RoleName, deleteOptions); err != nil {
		self.Log.Warningf("%s", err)

	}

	// Delete Role
	if err := self.Kubernetes.RbacV1().Roles(self.Namespace).Delete(self.Context, self.RoleName, deleteOptions); err != nil {
		self.Log.Warningf("%s", err)
	}

	return err
}
