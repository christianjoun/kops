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

//go:generate fitask -type=NetworkLoadBalancerAttachment

type NetworkLoadBalancerAttachment struct {

	// We use the Name tag to find the existing ELB, because we are (more or less) unrestricted when

	// it comes to tag values, but the LoadBalancerName is length limited

	Name *string

	Lifecycle *fi.Lifecycle
}

var _ fi.CompareWithID = &NetworkLoadBalancerAttachment{}

func (e *NetworkLoadBalancerAttachment) CompareWithID() *string {

	return e.Name

}

func (e *NetworkLoadBalancerAttachment) Find(c *fi.Context) (*NetworkLoadBalancerAttachment, error) {

	return nil, nil

}

var _ fi.HasAddress = &NetworkLoadBalancer{}

func (e *NetworkLoadBalancerAttachment) FindIPAddress(context *fi.Context) (*string, error) {

	return nil, nil

}

func (e *NetworkLoadBalancerAttachment) Run(c *fi.Context) error {

	// TODO: Make Normalize a standard method

	e.Normalize()

	return fi.DefaultDeltaRunMethod(e, c)

}

func (e *NetworkLoadBalancerAttachment) Normalize() {

	// We need to sort our arrays consistently, so we don't get spurious changes

	//sort.Stable(OrderSubnetsById(e.Subnets))

	//sort.Stable(OrderSecurityGroupsById(e.SecurityGroups))

}

func (s *NetworkLoadBalancerAttachment) CheckChanges(a, e, changes *NetworkLoadBalancerAttachment) error {

	return nil

}

func (_ *NetworkLoadBalancerAttachment) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *NetworkLoadBalancerAttachment) error {

	return nil

}
