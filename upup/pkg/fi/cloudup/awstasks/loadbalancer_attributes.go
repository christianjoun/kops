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

package awstasks

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/klog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
)

type LoadBalancerAccessLog struct {
	EmitInterval   *int64
	Enabled        *bool
	S3BucketName   *string
	S3BucketPrefix *string
}

func (_ *LoadBalancerAccessLog) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	return nil
}

type terraformLoadBalancerAccessLog struct {
	EmitInterval   *int64  `json:"interval,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	S3BucketName   *string `json:"bucket,omitempty"`
	S3BucketPrefix *string `json:"bucket_prefix,omitempty"`
}

type cloudformationLoadBalancerAccessLog struct {
	EmitInterval   *int64  `json:"EmitInterval,omitempty"`
	Enabled        *bool   `json:"Enabled,omitempty"`
	S3BucketName   *string `json:"S3BucketName,omitempty"`
	S3BucketPrefix *string `json:"S3BucketPrefix,omitempty"`
}

//type LoadBalancerAdditionalAttribute struct {
//	Key   *string
//	Value *string
//}
//
//func (_ *LoadBalancerAdditionalAttribute) GetDependencies(tasks map[string]fi.Task) []fi.Task {
//	return nil
//}

type LoadBalancerConnectionDraining struct {
	Enabled *bool
	Timeout *int64
}

func (_ *LoadBalancerConnectionDraining) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	return nil
}

type LoadBalancerCrossZoneLoadBalancing struct {
	Enabled *bool
}

func (_ *LoadBalancerCrossZoneLoadBalancing) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	return nil
}

type LoadBalancerConnectionSettings struct {
	IdleTimeout *int64
}

func (_ *LoadBalancerConnectionSettings) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	return nil
}

func findELBAttributes(cloud awsup.AWSCloud, LoadBalancerArn string) ([]*elbv2.LoadBalancerAttribute, error) {
	fmt.Println("****findELBAttributes:LoadBalancer_Attributes")

	request := &elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(LoadBalancerArn),
	}

	response, err := cloud.ELBV2().DescribeLoadBalancerAttributes(request)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}

	//we get back an array of attributes

	/*
	   Key *string `type:"string"`
	   Value *string `type:"string"`
	*/

	return response.Attributes, nil
}

func findELBAttributesOld(cloud awsup.AWSCloud, name string) (*elb.LoadBalancerAttributes, error) {
	fmt.Println("****findELBAttributes:LoadBalancer_Attributes")
	request := &elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: aws.String(name),
	}

	response, err := cloud.ELB().DescribeLoadBalancerAttributes(request)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}

	return response.LoadBalancerAttributes, nil
}

func (_ *LoadBalancer) modifyLoadBalancerAttributes(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer) error {
	//TODO: this might run this file w/o changing anything if only targetGroup needs changing..
	fmt.Println("****modifyLoadBalancerAttributes:LoadBalancer_Attributes")
	if changes.AccessLog == nil &&
		changes.ConnectionDraining == nil &&
		changes.ConnectionSettings == nil &&
		changes.CrossZoneLoadBalancing == nil {
		klog.V(4).Infof("No LoadBalancerAttribute changes; skipping update")
		return nil
	}

	loadBalancerName := fi.StringValue(e.LoadBalancerName)

	request := &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: e.LoadBalancerArn, //TODO: e or a or changes????
	}

	var attributes []*elbv2.LoadBalancerAttribute

	attribute := &elbv2.LoadBalancerAttribute{}
	attribute.Key = aws.String("load_balancing.cross_zone.enabled")
	if e.CrossZoneLoadBalancing == nil || e.CrossZoneLoadBalancing.Enabled == nil {
		attribute.Value = aws.String("false")
	} else {
		attribute.Value = aws.String("true")
	}
	attributes = append(attributes, attribute)

	request.Attributes = attributes

	klog.V(2).Infof("Configuring NLB attributes for NLB %q", loadBalancerName)

	response, err := t.Cloud.ELBV2().ModifyLoadBalancerAttributes(request)
	if err != nil {
		return fmt.Errorf("error configuring NLB attributes for NLB %q: %v", loadBalancerName, err)
	}

	klog.V(4).Infof("modified NLB attributes for NLB %q, response %+v", loadBalancerName, response)

	return nil
}

func (_ *LoadBalancer) modifyTargetGroupAttributes(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer) error {
	//TODO: this might run this file w/o changing anything if only loadbalancer attributes needs changing..
	fmt.Println("****modifyTargetGroupAttributes:LoadBalancer_Attributes")
	if changes.AccessLog == nil &&
		changes.ConnectionDraining == nil &&
		changes.ConnectionSettings == nil &&
		changes.CrossZoneLoadBalancing == nil {
		klog.V(4).Infof("No TargeGroupAttribute changes; skipping update")
		return nil
	}

	loadBalancerName := fi.StringValue(e.LoadBalancerName)
	request := &elbv2.ModifyTargetGroupAttributesInput{
		TargetGroupArn: e.TargetGroupArn, //TODO: e or a or changes????
	}

	var attributesTG []*elbv2.TargetGroupAttribute

	attribute := &elbv2.TargetGroupAttribute{}
	attribute.Key = aws.String("deregistration_delay.timeout_seconds")
	if e.ConnectionDraining == nil || e.ConnectionDraining.Timeout == nil {
		attribute.Value = aws.String("300")
	} else {
		attribute.Value = aws.String(strconv.Itoa(int(*e.ConnectionDraining.Timeout)))
	}

	attributesTG = append(attributesTG, attribute)

	request.Attributes = attributesTG

	responseTG, err := t.Cloud.ELBV2().ModifyTargetGroupAttributes(request)
	if err != nil {
		return fmt.Errorf("error configuring NLB target group attributes for NLB %q: %v", loadBalancerName, err)
	}

	klog.V(4).Infof("modified NLB target group attributes for NLB %q, response %+v", loadBalancerName, responseTG)

	return nil
}

func (_ *LoadBalancer) modifyLoadBalancerAttributesOld(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer) error {
	fmt.Println("****modifyLoadBalancerAttributes:LoadBalancer_Attributes")
	if changes.AccessLog == nil &&
		changes.ConnectionDraining == nil &&
		changes.ConnectionSettings == nil &&
		changes.CrossZoneLoadBalancing == nil {
		klog.V(4).Infof("No LoadBalancerAttribute changes; skipping update")
		return nil
	}

	loadBalancerName := fi.StringValue(e.LoadBalancerName)

	request := &elb.ModifyLoadBalancerAttributesInput{}
	request.LoadBalancerName = e.LoadBalancerName
	request.LoadBalancerAttributes = &elb.LoadBalancerAttributes{}

	// Setting mandatory attributes to default values if empty
	request.LoadBalancerAttributes.AccessLog = &elb.AccessLog{}
	if e.AccessLog == nil || e.AccessLog.Enabled == nil {
		request.LoadBalancerAttributes.AccessLog.Enabled = fi.Bool(false)
	}
	request.LoadBalancerAttributes.ConnectionDraining = &elb.ConnectionDraining{}
	if e.ConnectionDraining == nil || e.ConnectionDraining.Enabled == nil {
		request.LoadBalancerAttributes.ConnectionDraining.Enabled = fi.Bool(false)
	}
	if e.ConnectionDraining == nil || e.ConnectionDraining.Timeout == nil {
		request.LoadBalancerAttributes.ConnectionDraining.Timeout = fi.Int64(300)
	}
	request.LoadBalancerAttributes.ConnectionSettings = &elb.ConnectionSettings{}
	if e.ConnectionSettings == nil || e.ConnectionSettings.IdleTimeout == nil {
		request.LoadBalancerAttributes.ConnectionSettings.IdleTimeout = fi.Int64(60)
	}
	request.LoadBalancerAttributes.CrossZoneLoadBalancing = &elb.CrossZoneLoadBalancing{}
	if e.CrossZoneLoadBalancing == nil || e.CrossZoneLoadBalancing.Enabled == nil {
		request.LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled = fi.Bool(false)
	} else {
		request.LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled = e.CrossZoneLoadBalancing.Enabled
	}

	// Setting non mandatory values only if not empty

	// We don't map AdditionalAttributes (yet)
	//if len(e.AdditionalAttributes) != 0 {
	//	var additionalAttributes []*elb.AdditionalAttribute
	//	for index, additionalAttribute := range e.AdditionalAttributes {
	//		additionalAttributes[index] = &elb.AdditionalAttribute{
	//			Key:   additionalAttribute.Key,
	//			Value: additionalAttribute.Value,
	//		}
	//	}
	//	request.LoadBalancerAttributes.AdditionalAttributes = additionalAttributes
	//}

	if e.AccessLog != nil && e.AccessLog.EmitInterval != nil {
		request.LoadBalancerAttributes.AccessLog.EmitInterval = e.AccessLog.EmitInterval
	}
	if e.AccessLog != nil && e.AccessLog.S3BucketName != nil {
		request.LoadBalancerAttributes.AccessLog.S3BucketName = e.AccessLog.S3BucketName
	}
	if e.AccessLog != nil && e.AccessLog.S3BucketPrefix != nil {
		request.LoadBalancerAttributes.AccessLog.S3BucketPrefix = e.AccessLog.S3BucketPrefix
	}
	if e.ConnectionDraining != nil && e.ConnectionDraining.Timeout != nil {
		request.LoadBalancerAttributes.ConnectionDraining.Timeout = e.ConnectionDraining.Timeout
	}
	if e.ConnectionSettings != nil && e.ConnectionSettings.IdleTimeout != nil {
		request.LoadBalancerAttributes.ConnectionSettings.IdleTimeout = e.ConnectionSettings.IdleTimeout
	}

	klog.V(2).Infof("Configuring ELB attributes for ELB %q", loadBalancerName)

	response, err := t.Cloud.ELB().ModifyLoadBalancerAttributes(request)
	if err != nil {
		return fmt.Errorf("error configuring ELB attributes for ELB %q: %v", loadBalancerName, err)
	}

	klog.V(4).Infof("modified ELB attributes for ELB %q, response %+v", loadBalancerName, response)

	return nil
}
