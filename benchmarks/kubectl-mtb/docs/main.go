package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
)

const (
	// Location of the config files
	embedFolder string = "./test/benchmarks/"
)

// Structure of yaml (Used for README generation)
type Doc struct {
	ID              string                 `yaml:"id"`
	Title           string                 `yaml:"title"`
	BenchmarkType   string                 `yaml:"benchmarkType"`
	Category        string                 `yaml:"category"`
	Description     string                 `yaml:"description"`
	Remediation     string                 `yaml:"remediation"`
	ProfileLevel    int                    `yaml:"profileLevel"`
	AdditionalField map[string]interface{} `yaml:"additionalFields"`
}

// README template
const templ = `
# {{.Title}} <small>[{{.ID}}] </small>
**Profile Applicability:** 
{{.ProfileLevel}}
**Type:** 
{{.BenchmarkType}}
**Category:** 
{{.Category}} 
**Description:** 
{{.Description}} 
**Remediation:**
{{.Remediation}}
{{ range $key, $value := .AdditionalField }}
**{{ $key }}:** 
{{ $value }}
{{ end }}
`

func deleteFields(fieldname string, fieldmap map[string]interface{}) {

	delete(fieldmap, fieldname)

}

func main() {

	err := filepath.Walk(embedFolder, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			extension := filepath.Ext(path)
			if extension == ".yml" || extension == ".yaml" {
				b, err := ioutil.ReadFile(path)
				utils.CheckError(err)
				d := Doc{}
				// Unmarshall first time to get existing fields
				err = yaml.Unmarshal(b, &d)
				utils.CheckError(err)
				t := template.New("README template")
				t, err = t.Parse(templ)

				// Get directory of the config file
				dirPath := utils.GetDirectory(path, "/")

				//Check if Path exists
				_, err = utils.Exists(dirPath)
				utils.CheckError(err)

				f, err := os.Create(dirPath + "/README.md")
				utils.CheckError(err)

				// Write the output to the README file
				err = t.Execute(f, d)
				utils.CheckError(err)
				if err == nil {
					fmt.Println("README.md generated successfully")
				}

				err = f.Close()
				utils.CheckError(err)

			}
		}

		return nil
	})
	if err != nil {
		log.Fatal("Error walking through embed directory:", err)
	}

}
