# HNC User Guide
Authors: aludwin@google.com and other contributors from wg-multitenancy

Hierarchical Namespaces are a simple extension to Kubernetes namespaces that
makes it easy to manage groups of namespaces that share a common concept of
ownership. They are especially useful in clusters that are shared by multiple
teams, but the owners do not need to be people. For example, you might want to
make an Operator an owner of a set of namespaces.

This guide explains how to use hierarchical namespaces, explains some of the
concepts behind them for a more in-depth understanding, and covers some best
practices.

## Table of contents

* [How-to](how-to.md): common tasks when working with HNC
* [Concepts](concepts.md): learn more about the ideas behind HNC
* [Best practices](best-practices.md): learn about the best ways to deploy HNC
