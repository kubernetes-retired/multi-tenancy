/*
Copyright 2019 The Kubernetes Authors.

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
package main

import (
	"flag"
	"log"
	"os"

	_ "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/cmd/vcctl/subcmd"
)

func main() {
	flag.Parse()
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	yaml := createCmd.String("yaml", "", "path to the yaml file of the object that will be created.")
	vcKbCfg := createCmd.String("vckbcfg", "", "path to the kubeconfig that is used to access virtual cluster.")
	crtKbCfg := createCmd.String("kubeconfig", "", "path to the kubeconfig of the meta cluster.")

	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
	delYaml := deleteCmd.String("yaml", "", "path to the yaml file of the virtualcluster that will be deleted.")
	delKbCfg := deleteCmd.String("kubeconfig", "", "path to the kubeconfig file.")

	if len(os.Args) < 2 {
		log.Fatal("please use 'create' or 'delete' subcommand")
	}

	switch os.Args[1] {
	case "create":
		createCmd.Parse(os.Args[2:])
		// set flag --kubeconfig for pkg sigs.k8s.io/controller-runtime/pkg/client/config
		if *crtKbCfg != "" {
			if err := flag.Lookup("kubeconfig").Value.Set(*crtKbCfg); err != nil {
				log.Fatalf("fail to set flag kubeconfig: %s", err)
			}
		}
		if err := subcmd.Create(*yaml, *vcKbCfg); err != nil {
			log.Fatalf("fail to create object: %s", err)
		}
	case "delete":
		deleteCmd.Parse(os.Args[2:])
		// set flag --kubeconfig for pkg sigs.k8s.io/controller-runtime/pkg/client/config
		if *delKbCfg != "" {
			if err := flag.Lookup("kubeconfig").Value.Set(*delKbCfg); err != nil {
				log.Fatalf("fail to set flag kubeconfig: %s", err)
			}
		}
		if err := subcmd.Delete(*delYaml); err != nil {
			log.Fatalf("fail to delete object: %s", err)
		}
	default:
		log.Fatal("please use 'create' or 'delete' subcommand")
	}
}
