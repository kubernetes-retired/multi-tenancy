# Hacks

Gotta love this directory name, eh? Let's try to cover what's in here.

## HNC Build tools

Whenever possible, we've tried to include build tools like controller-gen in the
/vendors directory. In order to to force the Go tools _not_ to remove these
runtime dependencies from the go.mod file, this directory contains a fake
`tools.go` whose only purpose is to import these packages. Before you ask any
more questions about that, check out
https://stackoverflow.com/questions/52428230/how-do-go-modules-work-with-installable-commands
and hopefully it will answer them.

However, I wasn't able to get kustomize to fit in there, since it seems to have
dependencies which are not compatible with the rest of HNC. So I've just checked
in the Linux binary directly here. The exact version probably doesn't matter too
much; I just used whatever was most current and it worked.

## Templates

`boilerplate.go.txt` includes the Apache header. Kubebuilder put it here and it
seems like as good a place as any.

`krew-hierarchical-namespaces.yaml` is a template for the Krew `kubectl-hns`
plugin.

## CI

Other projects seem to put their presubmits, postsubmits etc here, so we did
too. See [here](../README.md#test-infrastructure) for where these are configured
in Prow.
