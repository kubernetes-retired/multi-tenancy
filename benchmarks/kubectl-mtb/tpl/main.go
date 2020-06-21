package main

import (
	"bufio"
	"fmt"
	"html/template"
	"log"
	"os"
	"reflect"
	"strings"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

type BenchmarkTemplate struct {
	embedFolder string
	PkgName     string
	FileName    string
}

var err error

func (bt *BenchmarkTemplate) Create() error {
	// Checking Benchmark Folder
	dir := bt.embedFolder + bt.FileName
	_, err := os.Stat(dir)

	// Create Benchmark Folder
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(dir, 0755)
		if errDir != nil {
			return err
		}
	}

	// create main.go
	mainFile, err := os.Create(fmt.Sprintf("%s/%s.go", bt.embedFolder+bt.FileName, bt.FileName))
	if err != nil {
		return err
	}
	defer mainFile.Close()

	mainTemplate := template.Must(template.New("main").Parse(string(BenchmarkPackage())))
	err = mainTemplate.Execute(mainFile, bt)
	if err != nil {
		return err
	}

	return nil
}

func (bt *BenchmarkTemplate) createConfig(b benchmark.Benchmark) error {
	// create config
	configFile, err := os.Create(fmt.Sprintf("%s/config.yaml", bt.embedFolder+bt.FileName))
	if err != nil {
		return err
	}
	defer configFile.Close()

	configTemplate := template.Must(template.New("main").Parse(string(ConfigYamlTemplate())))
	err = configTemplate.Execute(configFile, b)
	if err != nil {
		return err
	}

	return nil
}

func main() {

	// Initialize Benchmark config
	config := benchmark.Benchmark{}
	v := reflect.ValueOf(&config).Elem()
	typeOfConfig := v.Type()

	fmt.Println("This utility will walk you through adding a Benchmark.")

	// Loop on Config fields
	for i := 0; i < v.NumField(); i++ {

		// Take User Input as per Config field Types
		switch v.Field(i).Kind() {

		case reflect.String:
			fmt.Printf("%s: ", typeOfConfig.Field(i).Name)

			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			// Trime the trailing new line
			trimmedText := strings.TrimSuffix(text, "\n")
			v.Field(i).SetString(trimmedText)

		case reflect.Int:
			fmt.Printf("%s: ", typeOfConfig.Field(i).Name)

			var intInput int64
			fmt.Scan(&intInput)
			v.Field(i).SetInt(intInput)
		}
	}

	BenchmarkName := strings.ToLower(config.Title)
	UnderScoredBenchmarkName := strings.ReplaceAll(BenchmarkName, " ", "_")
	NoSpaceBenchmarkName := strings.ReplaceAll(BenchmarkName, " ", "")

	// Initialize the Benchmark Template
	bt := BenchmarkTemplate{
		embedFolder: "./test/benchmarks/",
		FileName:    UnderScoredBenchmarkName,
		PkgName:     NoSpaceBenchmarkName,
	}

	// Create the Benchmark
	err := bt.Create()
	if err != nil {
		log.Fatal(err)
	}

	// Create the Benchmark File
	err = bt.createConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s benchmark is successfully added under test/benchmarks/ directory :) \n", config.Title)
}
