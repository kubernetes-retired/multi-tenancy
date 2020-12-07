// See README.md for more information
package main

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// nsInfo defines all the information we need to know about a namespace
type nsInfo struct{
	parent string
	hasConfig bool
}

// forestType is the type of the forest: a map of namespace names -> nsInfo
type forestType map[string]*nsInfo

var (
	// forest is the hierarchical structure of all namespaces found in a structured Git repo, when run
	// in the "namespaces" directory.
	forest forestType

	// resourceList is populated by the call to framework.Command() in main() and includes all K8s
	// objects found in the directory.
	resourceList *framework.ResourceList
)

func main() {
	// Set up variables
	resourceList = &framework.ResourceList{}
	forest = forestType{}

	// Define and execute the only command
	cmd := framework.Command(resourceList, process)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// process reads all K8s objects, corrects them as necessary, and generates/fixes any necessary
// HierarchyConfiguration objects.
func process() error {
	if err := processItems(); err != nil {
		return err
	}

	if err := addConfigs(); err != nil {
		return err
	}

	return nil
}

// processItems reads each item, infers the desired hierarchy, and corrects any existing objects.
func processItems() error {
	for i, item := range resourceList.Items {
		// Get basic information about the object
		meta, err := item.GetMeta()
		if err != nil {
			return fmt.Errorf("cannot get meta for item %d: %w", i, err)
		}

		// path is the *file* path, which kpt adds into the object as a well-known annotation. From this
		// file path, we infer the hierarchy by looking at the path segments.
		path := meta.Annotations["config.kubernetes.io/path"]
		nnms, err := getNamespaces(path)
		if err != nil {
			return err
		}
		if nnms == nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "Processing %s...\n", path)

		// Update the inferred hierarchy
		if err := updateForest(nnms); err != nil {
			return fmt.Errorf("while inferring hierarchy from %q: %w", path, err)
		}

		if err := setNamespace(nnms, item, meta); err != nil {
			return fmt.Errorf("cannot set namespace for %s: %w", path, err)
		}

		// If this is a hierarchy config, make sure it's correct
		if err := processConfig(item, meta); err != nil {
			return fmt.Errorf("cannot update hierarchy config %s: %w", path, err)
		}
	}
	return nil
}

// getNamespaces takes the path and returns the namespace hierarchy, with the first element being
// the root namespaces and the last element being the leaf. If it returns an empty list, that means
// that this object is not in the `namespaces/` directory and should be ignored.
func getNamespaces(path string) ([]string, error) {
	segs := strings.Split(path, "/")

	// We only care about objects in the 'namespaces' directory.
	if segs[0] != "namespaces" {
		return nil, nil
	}

	// There shouldn't be any objects directly in the namespaces directory.
	if len(segs) < 3 {
		return nil, fmt.Errorf("file %q is directly under 'namespaces/' but should be in a namespace directory", path)
	}

	// After stripping off the 'namespaces/' and the filename, what remains is the list of namespaces
	// from root to leaf.
	return segs[1:len(segs)-1], nil
}

// updateForest updates the inferred forest based on the directory components in the item's
// filename.
func updateForest(nnms []string) error {
	pnm := "" // The root has no parent
	for _, nnm := range nnms {
		// Update our forest with the inferred hierarchy
		if ns, exists := forest[nnm]; exists {
			if ns.parent != pnm {
				return fmt.Errorf("namespace %q has conflicting parents: %q and %q\n", nnm, ns.parent, pnm)
			}
		} else {
			ns = &nsInfo{}
			ns.parent = pnm
			forest[nnm] = ns
		}

		// Update the parent for the next iteration
		pnm = nnm
	}
	return nil
}

// setNamespace updates the object with its namespace, unless the object *is* the namespace.
func setNamespace(nnms []string, item *yaml.RNode, meta yaml.ResourceMeta) error {
	// The namespace to set is the leaf namespace (the last element of the namespaces slice)
	nnm := nnms[len(nnms)-1]

	if meta.APIVersion == "v1" && meta.Kind == "Namespace" {
		// don't set the namespace field on the namespace object. However, do validate that the names
		// match!
		if meta.Name != nnm {
			return fmt.Errorf("namespace directory %q contains namespace config for %q", nnm, meta.Name)
		}
		return nil
	}

	return item.PipeE(yaml.SetK8sNamespace(nnm))
}

// processConfig marks the config as existing and corrects its .spec.parent field if it's wrong. It
// ignores the object if it's not a HierarchyConfiguration.
func processConfig(item *yaml.RNode, meta yaml.ResourceMeta) error {
	if meta.APIVersion != "hnc.x-k8s.io/v1alpha2" || meta.Kind != "HierarchyConfiguration" {
		return nil
	}
	ns := forest[meta.Namespace]
	ns.hasConfig = true

	spec, err := item.Pipe(yaml.Get("spec"))
	if err != nil {
		return fmt.Errorf("couldn't get spec: %w", err)
	}
	parent, err := spec.Pipe(yaml.Get("parent"))
	if err != nil {
		return fmt.Errorf("couldn't get spec.parent: %w", err)
	}

	oldParent := strings.TrimSpace(parent.MustString())
	if oldParent != ns.parent {
		fmt.Fprintf(os.Stderr, "HC config for %q has the wrong parent %q; updating to %q\n", meta.Namespace, oldParent, ns.parent)
		if err := spec.PipeE(yaml.SetField("parent", yaml.MustParse(ns.parent))); err != nil {
			return fmt.Errorf("couldn't update HC config: %s\n", err)
		}
	}
	return nil
}

// addConfigs looks through the forest for missing HierarchyConfigurations and adds any if they're
// missing.
func addConfigs() error {
	const template = `apiVersion: hnc.x-k8s.io/v1alpha2
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: %s
  annotations:
    config.kubernetes.io/path: %s
spec:
  parent: %s`

	for nm, ns := range forest {
		if ns.hasConfig || ns.parent == ""{
			// Don't generate configs if one already exists or for roots
			continue
		}

		// Build the filepath for this new yaml object based on its hierarchical path
		path := nm + "/hierarchyconfiguration.yaml"
		pnm := ns.parent
		for pnm != "" {
			path = pnm + "/" + path
			pnm = forest[pnm].parent
		}
		path = "namespaces/" + path

		// Generate the config from the template
		config := fmt.Sprintf(template, nm, path, ns.parent)
		n, err := yaml.Parse(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "HC config was:\n%s\n", config)
			return fmt.Errorf("couldn't generate hierarchy config for %q: %s\n", nm, err)
		}
		fmt.Fprintf(os.Stderr, "Generating %q\n", path)
		resourceList.Items = append(resourceList.Items, n)
	}

	return nil
}
