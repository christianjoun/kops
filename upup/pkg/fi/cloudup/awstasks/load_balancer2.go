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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/elb"

	"k8s.io/kops/upup/pkg/fi"

	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
)

// LoadBalancer manages an ELB.  We find the existing ELB using the Name tag.

//go:generate fitask -type=LoadBalancer2

type LoadBalancer2 struct {

	// We use the Name tag to find the existing ELB, because we are (more or less) unrestricted when

	// it comes to tag values, but the LoadBalancerName is length limited

	Name *string

	Lifecycle *fi.Lifecycle
}

var _ fi.CompareWithID = &LoadBalancer2{}

func (e *LoadBalancer2) CompareWithID() *string {

	return e.Name

}

type LoadBalancer2Listener struct {
	InstancePort int

	SSLCertificateID string
}

func (e *LoadBalancer2Listener) mapToAWS(loadBalancerPort int64) *elb.Listener {

	l := &elb.Listener{

		LoadBalancerPort: aws.Int64(loadBalancerPort),
	}

	return l

}

var _ fi.HasDependencies = &LoadBalancer2Listener{}

func (e *LoadBalancer2Listener) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	fmt.Println("!@@@@ Christian's nlb is in business")
	return nil

}

func (e *LoadBalancer2) Find(c *fi.Context) (*LoadBalancer2, error) {

	return nil, nil

}

var _ fi.HasAddress = &LoadBalancer2{}

func (e *LoadBalancer2) FindIPAddress(context *fi.Context) (*string, error) {

	return nil, nil

}

func (e *LoadBalancer2) Run(c *fi.Context) error {

	// TODO: Make Normalize a standard method

	e.Normalize()

	return fi.DefaultDeltaRunMethod(e, c)

}

func (e *LoadBalancer2) Normalize() {

	// We need to sort our arrays consistently, so we don't get spurious changes

	//sort.Stable(OrderSubnetsById(e.Subnets))

	//sort.Stable(OrderSecurityGroupsById(e.SecurityGroups))

}

func (s *LoadBalancer2) CheckChanges(a, e, changes *LoadBalancer2) error {

	return nil

}

func (_ *LoadBalancer2) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer2) error {

	return nil

}
