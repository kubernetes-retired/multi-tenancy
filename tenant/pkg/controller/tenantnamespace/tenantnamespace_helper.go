package tenantnamespace

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	cfgTemplate = `
kind: Config
apiVersion: v1
users:
- name: {{ .username }}
  user:
    token: {{ .token }}
clusters:
- name: {{ .cluster }}
  cluster:
    certificate-authority-data: {{ .ca }}
    server: {{ .master }}
contexts:
- context:
    cluster: {{ .cluster }}
    user: {{ .username }}
  name: default
current-context: default
preferences: {}
`
)

func GenerateCfgStr(clusterName string, ip string, ca, token []byte, username string) (string, error) {
	de := base64.StdEncoding.EncodeToString(token)
	r, _ := base64.StdEncoding.DecodeString(de)
	ctx := map[string]string{
		"ca":       base64.StdEncoding.EncodeToString(ca),
		"token":    string(r),
		"username": username,
		"master":   ip,
		"cluster":  clusterName,
	}

	return getTemplateContent(cfgTemplate, ctx)
}

// getTemplateContent fills out the kubeconfig templates based on the context
func getTemplateContent(kubeConfigTmpl string, context interface{}) (string, error) {
	t, tmplPrsErr := template.New("test").Parse(kubeConfigTmpl)
	if tmplPrsErr != nil {
		return "", tmplPrsErr
	}
	writer := bytes.NewBuffer([]byte{})
	if err := t.Execute(writer, context); nil != err {
		return "", err
	}

	return writer.String(), nil
}

func findSecretNameOfSA(c client.Client, saName string) (string, error) {
	//Get service account list, to fetch above sa because secret generation is async
	saList := corev1.ServiceAccountList{}
	if err := c.List(context.TODO(), &client.ListOptions{}, &saList); err != nil {
		return "", err
	}

	//Get secret name of tenant admin service account
	var saSecretName string
	for _, eachSA := range saList.Items {
		for _, each := range eachSA.Secrets {
			if strings.Contains(each.Name, saName) {
				saSecretName = each.Name
			}
		}
	}
	return saSecretName, nil
}

func getUniqueName(str string, a int) string{
	return fmt.Sprintf("%+v-%+v",str, int64(a))
}