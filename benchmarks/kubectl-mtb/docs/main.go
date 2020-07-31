package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	// Location of the config files
	embedFolder string = "./test/benchmarks/"
)

// Doc represents structure of yaml (Used for README generation)
type Doc struct {
	ID              string                 `yaml:"id"`
	Title           string                 `yaml:"title"`
	BenchmarkType   string                 `yaml:"benchmarkType"`
	Category        string                 `yaml:"category"`
	Description     string                 `yaml:"description"`
	Remediation     string                 `yaml:"remediation"`
	ProfileLevel    int                    `yaml:"profileLevel"`
	Rationale       string                 `yaml:"rationale"`
	Audit           string                 `yaml:"audit"`
	AdditionalField map[string]interface{} `yaml:"additionalFields"`
}

// README template
const templ = `# {{.Title}} <small>[{{.ID}}] </small>

**Profile Applicability:**

{{.ProfileLevel}} <br>

**Type:**

{{.BenchmarkType}} <br>

**Category:**

{{.Category}} <br>

**Description:**

{{.Description}} <br>

**Rationale:**

{{.Rationale}} <br>

**Audit:**

{{.Audit}} <br>

{{.Remediation}} <br>

{{ range $key, $value := .AdditionalField }}
**{{ $key }}:** 

{{ $value }} <br>

{{ end }}

`

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	return false, err
}

func getDirectory(path string, delimiter string) string {
	dir := strings.Split(path, delimiter)
	dir = dir[0 : len(dir)-1]
	dirPath := strings.Join(dir[:], "/")

	return dirPath
}

func deleteFields(fieldname string, fieldmap map[string]interface{}) {
	delete(fieldmap, fieldname)
}

func main() {
	err := filepath.Walk(embedFolder, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {

			extension := filepath.Ext(path)

			if extension == ".yml" || extension == ".yaml" {
				b, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}

				d := Doc{}
				// Unmarshall first time to get existing fields
				err = yaml.Unmarshal(b, &d)
				if err != nil {
					return err
				}

				// Unmarshall second time to add additonal fields
				err = yaml.Unmarshal(b, &d.AdditionalField)
				if err != nil {
					return err
				}

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
				dirPath := getDirectory(path, "/")

				//Check if Path exists
				_, err = exists(dirPath)
				if err != nil {
					return err
				}

				f, err := os.Create(dirPath + "/README.md")
				if err != nil {
					return err
				}

				// Write the output to the README file
				err = t.Execute(f, d)
				if err != nil {
					return err
				}

				err = f.Close()
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Fatal("Error walking through embed directory:", err)
	}

	fmt.Printf("Successfully Created README files. \xE2\x9C\x94 \n")
}
