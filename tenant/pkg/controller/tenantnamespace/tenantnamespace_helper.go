package tenantnamespace

import (
	"bytes"
	"encoding/base64"
	"html/template"
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


