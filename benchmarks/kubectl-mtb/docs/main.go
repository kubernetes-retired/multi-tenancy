package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util"
)

const (
	// Location of the config files
	embedFolder string = "./test/benchmarks/"
)

// Structure of yaml (Used for README generation)
type Doc struct {
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	BenchmarkType string `yaml:"benchmarkType"`
	Category      string `yaml:"category"`
	Description   string `yaml:"description"`
	Remediation   string `yaml:"remediation"`
	ProfileLevel  int    `yaml:"profileLevel"`
}

// README template
const templ = `
<!DOCTYPE html>
<html>
  <head>
    <title>README</title>
  </head>
  <body>
  <h2> {{.Title}} [{{.ID}}] </h2>
	<p>
		<b> Profile Applicability: </b> {{.ProfileLevel}} <br>
		<b> Type: </b> {{.BenchmarkType}} <br>
		<b> Category: </b> {{.Category}} <br>
		<b> Description: </b> {{.Description}} <br>
		<b> Remediation: </b> {{.Remediation}} <br>
	</p>
    
  </body>
</html>
`

func main() {

	err := filepath.Walk(embedFolder, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			extension := filepath.Ext(path)
			if extension == ".yml" || extension == ".yaml" {
				b, err := ioutil.ReadFile(path)
				util.CheckError(err)
				d := Doc{}
				err = yaml.Unmarshal(b, &d)
				util.CheckError(err)
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
