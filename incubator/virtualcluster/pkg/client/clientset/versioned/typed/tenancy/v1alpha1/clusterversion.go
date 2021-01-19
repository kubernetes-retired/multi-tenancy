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
// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	v1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	scheme "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned/scheme"
)

// ClusterVersionsGetter has a method to return a ClusterVersionInterface.
// A group's client should implement this interface.
type ClusterVersionsGetter interface {
	ClusterVersions() ClusterVersionInterface
}

// ClusterVersionInterface has methods to work with ClusterVersion resources.
type ClusterVersionInterface interface {
	Create(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.CreateOptions) (*v1alpha1.ClusterVersion, error)
	Update(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.UpdateOptions) (*v1alpha1.ClusterVersion, error)
	UpdateStatus(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.UpdateOptions) (*v1alpha1.ClusterVersion, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ClusterVersion, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ClusterVersionList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterVersion, err error)
	ClusterVersionExpansion
}

// clusterVersions implements ClusterVersionInterface
type clusterVersions struct {
	client rest.Interface
}

// newClusterVersions returns a ClusterVersions
func newClusterVersions(c *TenancyV1alpha1Client) *clusterVersions {
	return &clusterVersions{
		client: c.RESTClient(),
	}
}

// Get takes name of the clusterVersion, and returns the corresponding clusterVersion object, and an error if there is any.
func (c *clusterVersions) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClusterVersion, err error) {
	result = &v1alpha1.ClusterVersion{}
	err = c.client.Get().
		Resource("clusterversions").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterVersions that match those selectors.
func (c *clusterVersions) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClusterVersionList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.ClusterVersionList{}
	err = c.client.Get().
		Resource("clusterversions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterVersions.
func (c *clusterVersions) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("clusterversions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a clusterVersion and creates it.  Returns the server's representation of the clusterVersion, and an error, if there is any.
func (c *clusterVersions) Create(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.CreateOptions) (result *v1alpha1.ClusterVersion, err error) {
	result = &v1alpha1.ClusterVersion{}
	err = c.client.Post().
		Resource("clusterversions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterVersion).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a clusterVersion and updates it. Returns the server's representation of the clusterVersion, and an error, if there is any.
func (c *clusterVersions) Update(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.UpdateOptions) (result *v1alpha1.ClusterVersion, err error) {
	result = &v1alpha1.ClusterVersion{}
	err = c.client.Put().
		Resource("clusterversions").
		Name(clusterVersion.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterVersion).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *clusterVersions) UpdateStatus(ctx context.Context, clusterVersion *v1alpha1.ClusterVersion, opts v1.UpdateOptions) (result *v1alpha1.ClusterVersion, err error) {
	result = &v1alpha1.ClusterVersion{}
	err = c.client.Put().
		Resource("clusterversions").
		Name(clusterVersion.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterVersion).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the clusterVersion and deletes it. Returns an error if one occurs.
func (c *clusterVersions) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusterversions").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterVersions) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("clusterversions").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched clusterVersion.
func (c *clusterVersions) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterVersion, err error) {
	result = &v1alpha1.ClusterVersion{}
	err = c.client.Patch(pt).
		Resource("clusterversions").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
