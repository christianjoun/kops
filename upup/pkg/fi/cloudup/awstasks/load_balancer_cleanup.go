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
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/klog"
	"k8s.io/kops/upup/pkg/fi"

	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
)

// LoadBalancer manages an ELB.  We find the existing ELB using the Name tag.

//go:generate fitask -type=LoadBalancerCleanup

type LoadBalancerCleanup struct {

	// We use the Name tag to find the existing ELB, because we are (more or less) unrestricted when

	// it comes to tag values, but the LoadBalancerName is length limited

	Name         *string
	UseNLBForAPI *bool
	UseELBForAPI *bool
	AgNames      []*string
	NLBName      *string
	ELBName      *string

	Lifecycle *fi.Lifecycle
}

type deleteLoadBalancer struct {
	request *elb.DeleteLoadBalancerInput
}

var _ fi.Deletion = &deleteLoadBalancer{}

func (d *deleteLoadBalancer) TaskName() string {
	return "LoadBalancer"
}

func (d *deleteLoadBalancer) Item() string {
	return aws.StringValue(d.request.LoadBalancerName)
}

func (d *deleteLoadBalancer) Delete(t fi.Target) error {
	klog.V(2).Infof("deleting elb %v", d)

	awsTarget, ok := t.(*awsup.AWSAPITarget)
	if !ok {
		return fmt.Errorf("unexpected target type for deletion: %T", t)
	}

	name := aws.StringValue(d.request.LoadBalancerName)
	klog.V(2).Infof("Calling elb DeleteLoadBalancer for %s", name)

	_, err := awsTarget.Cloud.ELB().DeleteLoadBalancer(d.request)

	if err != nil {
		return fmt.Errorf("error deleting elb %s: %v", name, err)
	}

	return nil
}

func (d *deleteLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

type detachLoadBalancer struct {
	request *autoscaling.DetachLoadBalancersInput
}

var _ fi.Deletion = &detachLoadBalancer{}

func (d *detachLoadBalancer) TaskName() string {
	return "Autoscaling LoadBalancerAttachment"
}

func (d *detachLoadBalancer) Item() string {
	tmp := *d.request.LoadBalancerNames[0] + " -> " + *d.request.AutoScalingGroupName
	return aws.StringValue(&tmp)
}

func (d *detachLoadBalancer) Delete(t fi.Target) error {
	klog.V(2).Infof("deleting elb %v", d)

	awsTarget, ok := t.(*awsup.AWSAPITarget)
	if !ok {
		return fmt.Errorf("unexpected target type for deletion: %T", t)
	}

	name := aws.StringValue(d.request.AutoScalingGroupName)
	klog.V(2).Infof("Calling autoscaling Detach LoadBalancer for autoscaling group %s", name)

	_, err := awsTarget.Cloud.Autoscaling().DetachLoadBalancers(d.request)

	if err != nil {
		return fmt.Errorf("Error Detaching LoadBalancers from Autoscaling group : %v", err)
	}

	return nil
}

func (d *detachLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

type deleteTargetGroup struct {
	request *elbv2.DeleteTargetGroupInput
}

var _ fi.Deletion = &deleteTargetGroup{}

func (d *deleteTargetGroup) TaskName() string {
	return "TargetGroup"
}

func (d *deleteTargetGroup) Item() string {
	return aws.StringValue(d.request.TargetGroupArn)
}

func (d *deleteTargetGroup) Delete(t fi.Target) error {
	klog.V(2).Infof("deleting target group %v", d)

	awsTarget, ok := t.(*awsup.AWSAPITarget)
	if !ok {
		return fmt.Errorf("unexpected target type for deletion: %T", t)
	}

	name := aws.StringValue(d.request.TargetGroupArn)
	klog.V(2).Infof("Calling Nlb DeleteTargetGroup for %s", name)

	_, err := awsTarget.Cloud.ELBV2().DeleteTargetGroup(d.request)

	if err != nil {
		return fmt.Errorf("error Deleting TargetGroup from NLB: %v", err)
	}

	return nil
}

func (d *deleteTargetGroup) String() string {
	return d.TaskName() + "-" + d.Item()
}

type deleteNetworkLoadBalancer struct {
	request *elbv2.DeleteLoadBalancerInput
}

var _ fi.Deletion = &deleteNetworkLoadBalancer{}

func (d *deleteNetworkLoadBalancer) TaskName() string {
	return "LoadBalancer"
}

func (d *deleteNetworkLoadBalancer) Item() string {
	return aws.StringValue(d.request.LoadBalancerArn)
}

func (d *deleteNetworkLoadBalancer) Delete(t fi.Target) error {
	klog.V(2).Infof("deleting nlb %v", d)

	awsTarget, ok := t.(*awsup.AWSAPITarget)
	if !ok {
		return fmt.Errorf("unexpected target type for deletion: %T", t)
	}

	name := aws.StringValue(d.request.LoadBalancerArn)
	klog.V(2).Infof("Calling elb DeleteLoadBalancer for %s", name)

	_, err := awsTarget.Cloud.ELBV2().DeleteLoadBalancer(d.request)

	if err != nil {
		return fmt.Errorf("error deleting nlb %s: %v", name, err)
	}

	return nil
}

func (d *deleteNetworkLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

type detachNetworkLoadBalancer struct {
	request *autoscaling.DetachLoadBalancerTargetGroupsInput
}

var _ fi.Deletion = &detachNetworkLoadBalancer{}

func (d *detachNetworkLoadBalancer) TaskName() string {
	return "Autoscaling LoadBalancerTargetGroupAttachment"
}

func (d *detachNetworkLoadBalancer) Item() string {
	tmp := *d.request.TargetGroupARNs[0] + " -> " + *d.request.AutoScalingGroupName
	return aws.StringValue(&tmp)
}

func (d *detachNetworkLoadBalancer) Delete(t fi.Target) error {
	klog.V(2).Infof("deleting elb %v", d)

	awsTarget, ok := t.(*awsup.AWSAPITarget)
	if !ok {
		return fmt.Errorf("unexpected target type for deletion: %T", t)
	}

	name := aws.StringValue(d.request.AutoScalingGroupName)
	klog.V(2).Infof("Calling autoscaling Detach LoadBalancer for autoscaling group %s", name)

	_, err := awsTarget.Cloud.Autoscaling().DetachLoadBalancerTargetGroups(d.request)

	if err != nil {
		return fmt.Errorf("Error Detaching LoadBalancer TargetGroup from Autoscaling group : %v", err)
	}

	return nil
}

func (d *detachNetworkLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

func (e *LoadBalancerCleanup) FindELBDeletions(c *fi.Context) ([]fi.Deletion, error) {
	var removals []fi.Deletion

	cloud := c.Cloud.(awsup.AWSCloud)

	lb, err := FindLoadBalancerByNameTag(cloud, fi.StringValue(e.ELBName))

	if err != nil {
		return nil, err
	}

	if lb != nil {

		request := &elb.DeleteLoadBalancerInput{
			LoadBalancerName: lb.LoadBalancerName,
		}

		removals = append(removals, &deleteLoadBalancer{request: request})
		klog.V(2).Infof("will delete load balancer: %v", lb.LoadBalancerName)
	}

	for _, autoScalingGroupName := range e.AgNames {

		request := &autoscaling.DescribeLoadBalancersInput{
			AutoScalingGroupName: autoScalingGroupName,
		}
		response, err := cloud.Autoscaling().DescribeLoadBalancers(request)

		if err != nil {
			return nil, fmt.Errorf("Error querying Autoscaling to describe elb's : %v", err)
		}

		for _, LoadBalancerState := range response.LoadBalancers { //detach all elbs from autoscaling group

			loadBalancerName := LoadBalancerState.LoadBalancerName

			request := &autoscaling.DetachLoadBalancersInput{
				AutoScalingGroupName: autoScalingGroupName,
				LoadBalancerNames: []*string{
					loadBalancerName,
				},
			}

			removals = append(removals, &detachLoadBalancer{request: request})
			klog.V(2).Infof("will detach load balancer: %v from autoscalinggroup %v", loadBalancerName, autoScalingGroupName)
		}
	}

	return removals, nil
}

func (e *LoadBalancerCleanup) FindNLBDeletions(c *fi.Context) ([]fi.Deletion, error) {
	var removals []fi.Deletion

	cloud := c.Cloud.(awsup.AWSCloud)

	lb, err := FindNetworkLoadBalancerByNameTag(cloud, fi.StringValue(e.NLBName))

	if err != nil {
		return nil, err
	}

	if lb != nil {

		request := &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: lb.LoadBalancerArn,
		}

		removals = append(removals, &deleteNetworkLoadBalancer{request: request})
		klog.V(2).Infof("will delete network load balancer: %v", lb.LoadBalancerName)

		//TODO: findTargetGroupByNameTag: currently depends on there being a loadbalancer. other option is to delet targetgroups asssociated with autoscaling group. did this for speed
		{
			fmt.Printf("Describing Target Groups for loadBalancerArn : %v\n", lb.LoadBalancerArn)
			request := &elbv2.DescribeTargetGroupsInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			}
			response, err := cloud.ELBV2().DescribeTargetGroups(request)
			if err != nil {
				return nil, fmt.Errorf("error querying for NLB Target groups :%v", err)
			}

			if len(response.TargetGroups) == 0 {
				return nil, fmt.Errorf("Found no Target Groups for NLB don't think this is a normal condition :  %q", lb.LoadBalancerArn)
			}

			if len(response.TargetGroups) != 1 {
				return nil, fmt.Errorf("Found multiple Target groups for NLB with arn %q", lb.LoadBalancerArn)
			}

			targetGroupArn := response.TargetGroups[0].TargetGroupArn

			{ //Delete target group

				fmt.Printf("Deleting Target Group with arn : %v\n", targetGroupArn)

				request := &elbv2.DeleteTargetGroupInput{
					TargetGroupArn: targetGroupArn,
				}

				removals = append(removals, &deleteTargetGroup{request: request})
			}
		}

	}

	for _, autoScalingGroupName := range e.AgNames {

		request := &autoscaling.DescribeLoadBalancerTargetGroupsInput{
			AutoScalingGroupName: autoScalingGroupName,
		}

		response, err := cloud.Autoscaling().DescribeLoadBalancerTargetGroups(request)

		if err != nil {
			return nil, fmt.Errorf("Error querying Autoscaling to describe nlb's : %v", err)
		}

		for _, LoadBalancerState := range response.LoadBalancerTargetGroups { //detach all elbs from autoscaling group

			targetGroupArn := LoadBalancerState.LoadBalancerTargetGroupARN

			request := &autoscaling.DetachLoadBalancerTargetGroupsInput{
				AutoScalingGroupName: autoScalingGroupName,
				TargetGroupARNs: []*string{
					targetGroupArn,
				},
			}

			removals = append(removals, &detachNetworkLoadBalancer{request: request})
			klog.V(2).Infof("will detach targetGroup from autoscalinggroup %v", targetGroupArn, autoScalingGroupName)

		}
	}

	return removals, nil
}

func (e *LoadBalancerCleanup) FindDeletions(c *fi.Context) ([]fi.Deletion, error) {

	if *e.UseELBForAPI {
		return e.FindNLBDeletions(c)
	} else if *e.UseNLBForAPI {
		return e.FindELBDeletions(c)
	} else {
		nlbDeletions, err := e.FindNLBDeletions(c)
		if err != nil {
			return nil, err
		}
		elbDeletions, err := e.FindELBDeletions(c)
		if err != nil {
			return nil, err
		}
		return append(nlbDeletions, elbDeletions...), nil
	}

}

var _ fi.CompareWithID = &LoadBalancerCleanup{}

func (e *LoadBalancerCleanup) CompareWithID() *string {

	return e.Name
}

func (e *LoadBalancerCleanup) Find(c *fi.Context) (*LoadBalancerCleanup, error) {
	//avoid spurious mismatches
	actual := &LoadBalancerCleanup{}
	actual.Name = e.Name
	actual.Lifecycle = e.Lifecycle
	actual.AgNames = e.AgNames
	actual.UseNLBForAPI = e.UseNLBForAPI
	actual.UseELBForAPI = e.UseELBForAPI
	actual.NLBName = e.NLBName
	actual.ELBName = e.ELBName
	return actual, nil

}

var _ fi.HasAddress = &LoadBalancerCleanup{}

func (e *LoadBalancerCleanup) FindIPAddress(context *fi.Context) (*string, error) {

	return nil, nil

}

func (e *LoadBalancerCleanup) Run(c *fi.Context) error {

	// TODO: Make Normalize a standard method

	e.Normalize()

	return fi.DefaultDeltaRunMethod(e, c)

}

func (e *LoadBalancerCleanup) Normalize() {

	// We need to sort our arrays consistently, so we don't get spurious changes

	//sort.Stable(OrderSubnetsById(e.Subnets))

	//sort.Stable(OrderSecurityGroupsById(e.SecurityGroups))

}

func (s *LoadBalancerCleanup) CheckChanges(a, e, changes *LoadBalancerCleanup) error {

	return nil

}

func (_ *LoadBalancerCleanup) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *LoadBalancerCleanup) error {

	return nil

}
