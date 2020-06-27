package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util"
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
	delete(fieldmap, "description")

}

func main() {

	err := filepath.Walk(embedFolder, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			extension := filepath.Ext(path)
			if extension == ".yml" || extension == ".yaml" {
				b, err := ioutil.ReadFile(path)
				util.CheckError(err)
				d := Doc{}
				// Unmarshall first time to get existing fields
				err = yaml.Unmarshal(b, &d)
				util.CheckError(err)
				// Unmarshall second time to add additonal fields
				err = yaml.Unmarshal(b, &d.AdditionalField)
				util.CheckError(err)
				structVal := reflect.ValueOf(d)
				typeOfS := structVal.Type()

				values := make([]string, structVal.NumField())

				// iterate through struct to collect the fields
				for structField := 0; structField < structVal.NumField(); structField++ {
					if typeOfS.Field(structField).Name != "AdditionalField" {
						values[structField] = typeOfS.Field(structField).Tag.Get("yaml")
					}
				}
				// delete the existing fields which were added in the set of additional fields
				// during second unmarshalling
				for _, i := range values {
					deleteFields(i, d.AdditionalField)
				}

				t := template.New("README template")
				t, err = t.Parse(templ)

				// Get directory of the config file
				dirPath := util.GetDirectory(path, "/")

				//Check if Path exists
				_, err = util.Exists(dirPath)
				util.CheckError(err)

				f, err := os.Create(dirPath + "/README.md")
				util.CheckError(err)

				// Write the output to the README file
				err = t.Execute(f, d)
				util.CheckError(err)
				if err == nil {
					fmt.Println("README.md generated successfully")
				}

				err = f.Close()
				util.CheckError(err)

			}
		}

		return nil
	})
	if err != nil {
		log.Fatal("Error walking through embed directory:", err)
	}

}
