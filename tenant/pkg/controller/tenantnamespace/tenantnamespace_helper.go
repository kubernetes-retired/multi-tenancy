package tenantnamespace

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
)

const (
	kubeconfigTemplate = `
kind: Config
apiVersion: v1
users:
- name: {{ .username }}
  user:
    client-certificate-data: {{ .cert }}
    client-key-data: {{ .key }}
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

// generateKubeconfigUseCertAndKey generates kubeconfig based on the given crt/key pair
func GenerateKubeconfigUseCertAndKey(clusterName string, ips []string, caData, keyData, certData []byte, username string) (string, error) {
	urls := make([]string, 0, len(ips))
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("%+v", ip))
	}
	ctx := map[string]string{
		"ca":       base64.StdEncoding.EncodeToString(caData),
		"key":      base64.StdEncoding.EncodeToString(keyData),
		"cert":     base64.StdEncoding.EncodeToString(certData),
		"username": username,
		"master":   strings.Join(urls, ","),
		"cluster":  clusterName,
	}

	return getTemplateContent(kubeconfigTemplate, ctx)
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
