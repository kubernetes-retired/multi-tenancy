//go:generate go run generator.go
package main

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	importFileName string = "../../internal/kubectl-mtb/import.go"
	embedFolder    string = "../../test/benchmarks/"
)

// Define vars for build template
var tmpl = template.Must(template.New("").Parse(`package kubectl

// Code generated automatically; DO NOT EDIT.
import (
	{{- range $benchmark, $name := . }}
	_ "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/benchmarks/{{ $name }}"
	{{- end }}
)`),
)

func fmtByteSlice(s []byte) string {
	builder := strings.Builder{}

	for _, v := range s {
		builder.WriteString(fmt.Sprintf("%d,", int(v)))
	}

	return builder.String()
}

func main() {
	// Checking directory with files
	if _, err := os.Stat(embedFolder); os.IsNotExist(err) {
		log.Fatal("Configs directory does not exists!")
	}

	benchmarks := make(map[string]string)

	// Walking through embed directory
	filepath.Walk(embedFolder, func(path string, info os.FileInfo, err error) error {

		if info.IsDir() {
			BenchmarkName := filepath.ToSlash(strings.TrimPrefix(path, embedFolder))
			if BenchmarkName != "" {
				benchmarks[BenchmarkName] = BenchmarkName
			}
		}
		return nil
	})
	// Create import file
	f, err := os.Create(importFileName)
	if err != nil {
		log.Fatal("Error creating importfile file:", err)
	}
	defer f.Close()

	// Execute template
	if err = tmpl.Execute(f, benchmarks); err != nil {
		log.Fatal("Error executing template", err)
	}

	fmt.Printf("Import file generated successfully. \xE2\x9C\x94 \n")
}
