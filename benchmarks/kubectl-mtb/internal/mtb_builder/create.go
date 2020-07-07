/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package mtb_builder

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

var profileLevel int

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates the benchmark template.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		profileLevel, _ = cmd.Flags().GetInt("profile-level")
		if profileLevel == 0 {
			return fmt.Errorf("profile-level must be set via --profile-level or -p (1, 2, 3)")
		}

		return nil
	},

	RunE: func(cmd *cobra.Command, args []string) error {

		// Initialize Benchmark config
		config := benchmark.Benchmark{}

		if len(args) == 0 {
			return fmt.Errorf("Provide Benchmark Name \nExample: mtb-builder create block host IPC --profile-level 1")
		}

		config.Title = strings.Join(args[:], " ")
		config.ProfileLevel = profileLevel

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
			return err
		}

		// Create the Benchmark File
		err = bt.createConfig(config)
		if err != nil {
			return err
		}

		// Create the Benchmark File
		err = bt.createTest()
		if err != nil {
			return err
		}

		fmt.Printf(
			`%s benchmark is successfully added under test/benchmarks/ directory.
Next Steps to complete your benchmark are :-
1 - Complete config.yaml according to the benchmark specification.
2 - Complete the Prerun and Run functions in %s.go
	2.1 - PreRun - Validates the benchmark running condition
	2.2 - Run - Implementation of Benchmark
3 - Write the Unit tests in %s_test.go
`, config.Title, UnderScoredBenchmarkName, UnderScoredBenchmarkName)

		return nil
	},
}

var err error

// BenchmarkTemplate consist the template information i.e. used to create dirs and files
type BenchmarkTemplate struct {
	embedFolder string
	PkgName     string
	FileName    string
}

// Create function creates the benchmark folder and go file
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

	mainFile, err := os.Create(fmt.Sprintf("%s/%s.go", bt.embedFolder+bt.FileName, bt.FileName))
	if err != nil {
		return err
	}
	defer mainFile.Close()

	mainTemplate := template.Must(template.New("main").Parse(string(BenchmarkFileTemplate())))
	err = mainTemplate.Execute(mainFile, bt)
	if err != nil {
		return err
	}

	return nil
}

// Createconfig function creates the benchmark config file
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

// Createconfig function creates the benchmark config file
func (bt *BenchmarkTemplate) createTest() error {
	// create config
	testFile, err := os.Create(fmt.Sprintf("%s/%s_test.go", bt.embedFolder+bt.FileName, bt.FileName))
	if err != nil {
		return err
	}
	defer testFile.Close()

	configTemplate := template.Must(template.New("main").Parse(string(BenchmarkTestTemplate())))
	err = configTemplate.Execute(testFile, bt)
	if err != nil {
		return err
	}

	return nil
}

func newCreateCmd() *cobra.Command {
	profileLevel := rootCmd.PersistentFlags()
	profileLevel.IntP("profile-level", "p", 0, "Profile Level of the benchmark.")
	cobra.MarkFlagRequired(profileLevel, "profile-level")

	return createCmd
}
