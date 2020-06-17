package benchmark

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/russross/blackfriday/v2"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util"
)

// Benchmark consists the benchmark information like benchmark id, name, remediation etc.
type Benchmark struct {
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	BenchmarkType string `yaml:"benchmarkType"`
	Category      string `yaml:"category"`
	Description   string `yaml:"description"`
	Remediation   string `yaml:"remediation"`
	ProfileLevel  int    `yaml:"profileLevel"`
	Run           func(string, *kubernetes.Clientset, *kubernetes.Clientset) (bool, error)
}

// ReadConfig reads the yaml representation of struct from []file
func (b *Benchmark) ReadConfig(file []byte, path string) error {
	if err := yaml.Unmarshal(file, b); err != nil {
		return err
	}

	if b == nil {
		return errors.New("Please fill in a valid/non-empty yaml file")
	}

	output := blackfriday.Run(file)
	testDir := util.GetDirectory(path, "/")
	filePath, _ := filepath.Abs("./test/benchmarks/" + testDir + "/README.md")
	f, err := os.Create(filePath)
	if err != nil {
		fmt.Println(err)
	}
	_, err = f.Write(output)
	if err != nil {
		fmt.Println(err)
		f.Close()
	} else {
		fmt.Println("README.md generated successfully")
	}

	err = f.Close()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}
