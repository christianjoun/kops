/*

Copyright 2016 The Kubernetes Authors.



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

package awstasks

import (
	"k8s.io/kops/upup/pkg/fi"

	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
)

// LoadBalancer manages an ELB.  We find the existing ELB using the Name tag.

//go:generate fitask -type=NetworkLoadBalancerCleanup

type NetworkLoadBalancerCleanup struct {

	// We use the Name tag to find the existing ELB, because we are (more or less) unrestricted when

	// it comes to tag values, but the LoadBalancerName is length limited

	Name *string

	Lifecycle *fi.Lifecycle
}

var _ fi.CompareWithID = &NetworkLoadBalancerCleanup{}

func (e *NetworkLoadBalancerCleanup) CompareWithID() *string {

	return e.Name

}

func (e *NetworkLoadBalancerCleanup) Find(c *fi.Context) (*NetworkLoadBalancerCleanup, error) {

	return nil, nil

}

var _ fi.HasAddress = &NetworkLoadBalancer{}

func (e *NetworkLoadBalancerCleanup) FindIPAddress(context *fi.Context) (*string, error) {

	return nil, nil

}

func (e *NetworkLoadBalancerCleanup) Run(c *fi.Context) error {

	// TODO: Make Normalize a standard method

	e.Normalize()

	return fi.DefaultDeltaRunMethod(e, c)

}

func (e *NetworkLoadBalancerCleanup) Normalize() {

	// We need to sort our arrays consistently, so we don't get spurious changes

	//sort.Stable(OrderSubnetsById(e.Subnets))

	//sort.Stable(OrderSecurityGroupsById(e.SecurityGroups))

}

func (s *NetworkLoadBalancerCleanup) CheckChanges(a, e, changes *NetworkLoadBalancerCleanup) error {

	return nil

}

func (_ *NetworkLoadBalancerCleanup) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *NetworkLoadBalancerCleanup) error {

	return nil

}
