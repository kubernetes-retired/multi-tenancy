package main

func BenchmarkPackage() []byte {
	return []byte(`package {{ .PkgName }}

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("{{ .FileName }}/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b);
}
	`)
}

func ConfigYamlTemplate() []byte {
	return []byte(
		`id: {{ .ID }}
title: {{ .Title }}
benchmarkType: {{ .BenchmarkType }}
category: {{ .Category }} 
description: {{ .Description }}
remediation: {{ .Remediation }}
profileLevel: {{ .ProfileLevel  }}
`)
}
