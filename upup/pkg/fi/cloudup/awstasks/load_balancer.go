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
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"k8s.io/klog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/cloudformation"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
	"k8s.io/kops/util/pkg/slice"
)

// LoadBalancer manages an ELB.  We find the existing ELB using the Name tag.

//go:generate fitask -type=LoadBalancer
type LoadBalancer struct {
	// We use the Name tag to find the existing ELB, because we are (more or less) unrestricted when
	// it comes to tag values, but the LoadBalancerName is length limited
	Name      *string
	Lifecycle *fi.Lifecycle

	// LoadBalancerName is the name in ELB, possibly different from our name
	// (ELB is restricted as to names, so we have limited choices!)
	// We use the Name tag to find the existing ELB.
	LoadBalancerName *string
	LoadBalancerArn  *string
	TargetGroupArn   *string
	ListenerArns     []*string

	DNSName      *string
	HostedZoneId *string

	Subnets        []*Subnet
	SecurityGroups []*SecurityGroup

	Listeners map[string]*LoadBalancerListener

	Scheme *string
	Type   *string

	HealthCheck            *LoadBalancerHealthCheck
	AccessLog              *LoadBalancerAccessLog
	ConnectionDraining     *LoadBalancerConnectionDraining //TODO: remove
	ConnectionSettings     *LoadBalancerConnectionSettings //TODO: Remove
	CrossZoneLoadBalancing *LoadBalancerCrossZoneLoadBalancing
	SSLCertificateID       string

	Tags               map[string]string
	VPC                *VPC
	DeletionProtection *LoadBalancerDeletionProtection
	ProxyProtocolV2    *TargetGroupProxyProtocolV2
	Stickiness         *TargetGroupStickiness
	DeregistationDelay *TargetGroupDeregistrationDelay
	AgNames            []*string
	UseNLBForAPI       *bool
}

var _ fi.CompareWithID = &LoadBalancer{}

func (e *LoadBalancer) CompareWithID() *string {
	fmt.Println("**** CompareWithID")
	return e.Name
}

type LoadBalancerListener struct {
	InstancePort     int //TODO: Change this to LoadBalancerPort
	SSLCertificateID string
}

func (e *LoadBalancerListener) mapToAWS(loadBalancerPort int64) *elb.Listener {
	fmt.Println("**** mapToAWS")
	l := &elb.Listener{
		LoadBalancerPort: aws.Int64(loadBalancerPort),
		InstancePort:     aws.Int64(int64(e.InstancePort)),
	}

	if e.SSLCertificateID != "" {
		l.Protocol = aws.String("SSL")
		l.InstanceProtocol = aws.String("SSL")
		l.SSLCertificateId = aws.String(e.SSLCertificateID)
	} else {
		l.Protocol = aws.String("TCP")
		l.InstanceProtocol = aws.String("TCP")
	}

	return l
}

//TODO:
/*func (e *LoadBalancerListener) mapToAWS2(loadBalancerPort int64) *elbv2.CreateListenerInput {
	fmt.Println("**** mapToAWS")
	l := &elbv2.CreateListenerInput{
		LoadBalancerPort: aws.Int64(loadBalancerPort),
		InstancePort:     aws.Int64(int64(e.InstancePort)),
	}

	if e.SSLCertificateID != "" {
		request := &elbv2.AddListenerCertificatesInput{}
		request.SetListenerArn()
		l.AddListenerCertificates()
		l.Protocol = aws.String("SSL")
		l.InstanceProtocol = aws.String("SSL")
		l.SSLCertificateId = aws.String(e.SSLCertificateID)
	} else {
		l.Protocol = aws.String("TCP")
		l.InstanceProtocol = aws.String("TCP")
	}

	return l
}*/

var _ fi.HasDependencies = &LoadBalancerListener{}

func (e *LoadBalancerListener) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	fmt.Println("**** GetDependencies")
	return nil
}

type deleteLoadBalancer struct {
	request *elbv2.DeleteLoadBalancerInput
}

var _ fi.Deletion = &deleteLoadBalancer{}

func (d *deleteLoadBalancer) TaskName() string {
	return "LoadBalancer"
}

func (d *deleteLoadBalancer) Item() string {
	return aws.StringValue(d.request.LoadBalancerArn)
}

func (d *deleteLoadBalancer) Delete(t fi.Target) error {
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

func (d *deleteLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

type detachLoadBalancer struct {
	request *autoscaling.DetachLoadBalancerTargetGroupsInput
}

var _ fi.Deletion = &detachLoadBalancer{}

func (d *detachLoadBalancer) TaskName() string {
	return "Autoscaling LoadBalancerTargetGroupAttachment"
}

func (d *detachLoadBalancer) Item() string {
	tmp := *d.request.TargetGroupARNs[0] + " -> " + *d.request.AutoScalingGroupName
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

	_, err := awsTarget.Cloud.Autoscaling().DetachLoadBalancerTargetGroups(d.request)

	if err != nil {
		return fmt.Errorf("Error Detaching LoadBalancer TargetGroup from Autoscaling group : %v", err)
	}

	return nil
}

func (d *detachLoadBalancer) String() string {
	return d.TaskName() + "-" + d.Item()
}

func (e *LoadBalancer) FindDeletions(c *fi.Context) ([]fi.Deletion, error) {
	var removals []fi.Deletion

	cloud := c.Cloud.(awsup.AWSCloud)

	if !*e.UseNLBForAPI {
		lb, err := FindLoadBalancerByNameTag(cloud, fi.StringValue(e.Name))

		if err != nil {
			return nil, err
		}

		if lb != nil {

			request := &elbv2.DeleteLoadBalancerInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			}

			removals = append(removals, &deleteLoadBalancer{request: request})
			klog.V(2).Infof("will delete load balancer: %v", lb.LoadBalancerName)
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

				removals = append(removals, &detachLoadBalancer{request: request})
				klog.V(2).Infof("will detach load balancer: %v from autoscalinggroup %v", lb.LoadBalancerArn, autoScalingGroupName)
			}
		}
	}

	return removals, nil
}

// func findLoadBalancerByLoadBalancerName2(cloud awsup.AWSCloud, loadBalancerName string) (*elbv2.Load .LoadBalancerDescription, error) {
// 	fmt.Println("")
// 	request := &elbv2.DescribeLoadBalancersInput{
// 		LoadBalancerNames: []*string{&loadBalancerName},
// 	}
// 	return request
// }

func findTargetGroupByLoadBalancerName(cloud awsup.AWSCloud, loadBalancerNameTag string) (*elbv2.TargetGroup, error) {
	fmt.Println("***** findTargetGroupByLoadBalancerName")

	lb, err := FindLoadBalancerByNameTag(cloud, loadBalancerNameTag)
	if err != nil {
		return nil, fmt.Errorf("Can't locate NLB with Name Tag %v in findTargetGroupByLoadBalancerName : %v", loadBalancerNameTag, err)
	}

	if lb == nil { //should this be an error?
		return nil, nil
	}

	request := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: lb.LoadBalancerArn,
	}

	response, err := cloud.ELBV2().DescribeTargetGroups(request)

	if err != nil {
		return nil, fmt.Errorf("Error retrieving target groups for loadBalancerName %v with err : %v", loadBalancerNameTag, err)
	}

	if len(response.TargetGroups) != 1 {
		return nil, fmt.Errorf("Wrong # of target groups returned in findTargetGroupByLoadBalancerName for name %v", loadBalancerNameTag)
	}

	return response.TargetGroups[0], nil
}

func findLoadBalancerByLoadBalancerName(cloud awsup.AWSCloud, loadBalancerName string) (*elb.LoadBalancerDescription, error) {
	fmt.Println("**** findLoadBalancerByLoadBalancerName")
	request := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{&loadBalancerName},
	}
	found, err := describeLoadBalancers(cloud, request, func(lb *elb.LoadBalancerDescription) bool {
		// TODO: Filter by cluster?

		if aws.StringValue(lb.LoadBalancerName) == loadBalancerName {
			return true
		}

		klog.Warningf("Got ELB with unexpected name: %q", aws.StringValue(lb.LoadBalancerName))
		return false
	})

	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}

		return nil, fmt.Errorf("error listing ELBs: %v", err)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple ELBs with name %q", loadBalancerName)
	}

	return found[0], nil
}

//The load balancer name 'api.renamenlbcluster.k8s.local' can only contain characters that are alphanumeric characters and hyphens(-)\n\tstatus code: 400,
func findLoadBalancerByLoadBalancerName2(cloud awsup.AWSCloud, loadBalancerName string) (*elbv2.LoadBalancer, error) {
	fmt.Println("**** findLoadBalancerByLoadBalancerName2")
	request := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{&loadBalancerName},
	}
	found, err := describeLoadBalancers2(cloud, request, func(lb *elbv2.LoadBalancer) bool {
		// TODO: Filter by cluster?

		if aws.StringValue(lb.LoadBalancerName) == loadBalancerName {
			return true
		}

		klog.Warningf("Got NLB with unexpected name: %q", aws.StringValue(lb.LoadBalancerName))
		return false
	})

	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}

		return nil, fmt.Errorf("error listing NLBs: %v", err)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple NLBs with name %q", loadBalancerName)
	}

	return found[0], nil
}

func findLoadBalancerByAlias(cloud awsup.AWSCloud, alias *route53.AliasTarget) (*elbv2.LoadBalancer, error) {
	//TODO: test this function works as expected.
	fmt.Println("**** findLoadBalancerByAlias")
	// TODO: Any way to avoid listing all ELBs?
	//request := &elb.DescribeLoadBalancersInput{}
	request := &elbv2.DescribeLoadBalancersInput{}

	dnsName := aws.StringValue(alias.DNSName)
	matchDnsName := strings.TrimSuffix(dnsName, ".")
	if matchDnsName == "" {
		return nil, fmt.Errorf("DNSName not set on AliasTarget")
	}

	matchHostedZoneId := aws.StringValue(alias.HostedZoneId)

	found, err := describeLoadBalancers2(cloud, request, func(lb *elbv2.LoadBalancer) bool {
		// TODO: Filter by cluster?

		if matchHostedZoneId != aws.StringValue(lb.CanonicalHostedZoneId) {
			return false
		}

		lbDnsName := aws.StringValue(lb.DNSName)
		lbDnsName = strings.TrimSuffix(lbDnsName, ".")
		return lbDnsName == matchDnsName || "dualstack."+lbDnsName == matchDnsName
	})

	if err != nil {
		return nil, fmt.Errorf("error listing NLBs: %v", err)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple NLBs with DNSName %q", dnsName)
	}

	return found[0], nil
}

func findLoadBalancerByAliasOld(cloud awsup.AWSCloud, alias *route53.AliasTarget) (*elb.LoadBalancerDescription, error) {
	fmt.Println("**** findLoadBalancerByAlias")
	// TODO: Any way to avoid listing all ELBs?
	request := &elb.DescribeLoadBalancersInput{}

	dnsName := aws.StringValue(alias.DNSName)
	matchDnsName := strings.TrimSuffix(dnsName, ".")
	if matchDnsName == "" {
		return nil, fmt.Errorf("DNSName not set on AliasTarget")
	}

	matchHostedZoneId := aws.StringValue(alias.HostedZoneId)

	found, err := describeLoadBalancers(cloud, request, func(lb *elb.LoadBalancerDescription) bool {
		// TODO: Filter by cluster?

		if matchHostedZoneId != aws.StringValue(lb.CanonicalHostedZoneNameID) {
			return false
		}

		lbDnsName := aws.StringValue(lb.DNSName)
		lbDnsName = strings.TrimSuffix(lbDnsName, ".")
		return lbDnsName == matchDnsName || "dualstack."+lbDnsName == matchDnsName
	})

	if err != nil {
		return nil, fmt.Errorf("error listing ELBs: %v", err)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple ELBs with DNSName %q", dnsName)
	}

	return found[0], nil
}

//findNaemTag= e.Name (api.clusterName())
func FindLoadBalancerByNameTag(cloud awsup.AWSCloud, findNameTag string) (*elbv2.LoadBalancer, error) {
	fmt.Printf("**** FindLoadBalancerByNameTag %v\n", findNameTag)
	// TODO: Any way around this?
	klog.V(2).Infof("Listing all ELBs for findLoadBalancerByNameTag")

	request := &elbv2.DescribeLoadBalancersInput{}
	// ELB DescribeTags has a limit of 20 names, so we set the page size here to 20 also
	request.PageSize = aws.Int64(20)

	var found []*elbv2.LoadBalancer

	var innerError error
	err := cloud.ELBV2().DescribeLoadBalancersPages(request, func(p *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
		if len(p.LoadBalancers) == 0 {
			return true
		}

		// TODO: Filter by cluster?

		var names []string
		nameToELB := make(map[string]*elbv2.LoadBalancer)
		for _, elb := range p.LoadBalancers {
			name := aws.StringValue(elb.LoadBalancerName)
			nameToELB[name] = elb
			names = append(names, name)
		}

		var arns []string
		arnToELB := make(map[string]*elbv2.LoadBalancer)
		for _, elb := range p.LoadBalancers {
			arn := aws.StringValue(elb.LoadBalancerArn)
			arnToELB[arn] = elb
			arns = append(arns, arn)
		}

		//fmt.Printf(" describeLoadbalancerPages names = %s\n", names)
		tagMap, err := describeLoadBalancerTags(cloud, arns)
		if err != nil {
			innerError = err
			return false
		}

		//fmt.Printf("tagMap from describeLoadBalancerTags = %v\n", tagMap)
		for loadBalancerArn, tags := range tagMap {
			//fmt.Printf("tags = %s\n", tags)
			name, foundNameTag := awsup.FindELBV2Tag(tags, "Name")
			if !foundNameTag || name != findNameTag {
				//fmt.Printf("foundNameTag=%+v, name=%+v, findNameTag=%+v, \n", foundNameTag, name, findNameTag)
				continue
			}
			//fmt.Printf("found our ELB, the ARN we want is +%v\n", loadBalancerArn)
			elb, ok := arnToELB[loadBalancerArn]
			if !ok {
				panic("something wrong")
			}
			found = append(found, elb)
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("error describing LoadBalancers: %v", err)
	}
	if innerError != nil {
		return nil, fmt.Errorf("error describing LoadBalancers: %v", innerError)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple ELBs with Name %q", findNameTag)
	}

	return found[0], nil
}

/*func FindLoadBalancerByNameTag(cloud awsup.AWSCloud, findNameTag string) (*elb.LoadBalancerDescription, error) {
	fmt.Printf("**** FindLoadBalancerByNameTag %v\n", findNameTag)
	// TODO: Any way around this?
	klog.V(2).Infof("Listing all ELBs for findLoadBalancerByNameTag")

	request := &elb.DescribeLoadBalancersInput{}
	// ELB DescribeTags has a limit of 20 names, so we set the page size here to 20 also
	request.PageSize = aws.Int64(20)

	var found []*elb.LoadBalancerDescription

	var innerError error
	err := cloud.ELB().DescribeLoadBalancersPages(request, func(p *elb.DescribeLoadBalancersOutput, lastPage bool) bool {
		if len(p.LoadBalancerDescriptions) == 0 {
			return true
		}

		// TODO: Filter by cluster?

		var names []string
		nameToELB := make(map[string]*elb.LoadBalancerDescription)
		for _, elb := range p.LoadBalancerDescriptions {
			name := aws.StringValue(elb.LoadBalancerName)
			nameToELB[name] = elb
			names = append(names, name)
		}

		tagMap, err := describeLoadBalancerTags(cloud, names)
		if err != nil {
			innerError = err
			return false
		}

		for loadBalancerName, tags := range tagMap {
			name, foundNameTag := awsup.FindELBTag(tags, "Name")
			if !foundNameTag || name != findNameTag {
				continue
			}

			elb := nameToELB[loadBalancerName]
			found = append(found, elb)
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("error describing LoadBalancers: %v", err)
	}
	if innerError != nil {
		return nil, fmt.Errorf("error describing LoadBalancers: %v", innerError)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple ELBs with Name %q", findNameTag)
	}

	return found[0], nil
}*/

func describeLoadBalancers(cloud awsup.AWSCloud, request *elb.DescribeLoadBalancersInput, filter func(*elb.LoadBalancerDescription) bool) ([]*elb.LoadBalancerDescription, error) {
	fmt.Println("**** describeLoadBalancers")
	var found []*elb.LoadBalancerDescription
	err := cloud.ELB().DescribeLoadBalancersPages(request, func(p *elb.DescribeLoadBalancersOutput, lastPage bool) (shouldContinue bool) {
		for _, lb := range p.LoadBalancerDescriptions {
			if filter(lb) {
				found = append(found, lb)
			}
		}

		return true
	})

	if err != nil {
		return nil, fmt.Errorf("error listing ELBs: %v", err)
	}

	return found, nil
}

func describeLoadBalancers2(cloud awsup.AWSCloud, request *elbv2.DescribeLoadBalancersInput, filter func(*elbv2.LoadBalancer) bool) ([]*elbv2.LoadBalancer, error) {
	fmt.Println("**** describeLoadBalancers2")
	var found []*elbv2.LoadBalancer
	err := cloud.ELBV2().DescribeLoadBalancersPages(request, func(p *elbv2.DescribeLoadBalancersOutput, lastPage bool) (shouldContinue bool) {
		for _, lb := range p.LoadBalancers {
			if filter(lb) {
				found = append(found, lb)
			}
		}

		return true
	})

	if err != nil {
		return nil, fmt.Errorf("error listing NLBs: %v", err)
	}

	return found, nil
}

//TODO rename this function cause it works on targeGroups too?
//can only request loadbalancertags given a loadbalancerArn
//returns arns:Tags
func describeLoadBalancerTags(cloud awsup.AWSCloud, loadBalancerArns []string) (map[string][]*elbv2.Tag, error) {
	fmt.Println("**** describeLoadBalancerTags")
	//fmt.Printf("Querying ELB tags for %s", loadBalancerArns)
	for _, name := range loadBalancerArns {
		fmt.Printf("loadBalancerArns = %v\n", name)
	}
	// TODO: Filter by cluster?

	request := &elbv2.DescribeTagsInput{}
	request.ResourceArns = aws.StringSlice(loadBalancerArns)

	// TODO: Cache?
	klog.V(2).Infof("Querying ELBV2 api for tags for %s", loadBalancerArns)
	response, err := cloud.ELBV2().DescribeTags(request)
	if err != nil {
		fmt.Println("*** here is our error i hope ***")
		return nil, err
	}

	tagMap := make(map[string][]*elbv2.Tag)
	for _, tagset := range response.TagDescriptions {
		tagMap[aws.StringValue(tagset.ResourceArn)] = tagset.Tags
	}
	return tagMap, nil
}

func (e *LoadBalancer) Find(c *fi.Context) (*LoadBalancer, error) {
	fmt.Printf("**** Find - e.name = %v\n", *e.Name)
	cloud := c.Cloud.(awsup.AWSCloud)

	//e.Name = "api." + b.ClusterName()
	lb, err := FindLoadBalancerByNameTag(cloud, fi.StringValue(e.Name))
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, nil
	}

	loadBalancerArn := lb.LoadBalancerArn
	var targetGroupArn *string
	fmt.Println("I suspect shouldn't go past here because there isn't one")
	actual := &LoadBalancer{}
	actual.Name = e.Name
	actual.Lifecycle = e.Lifecycle
	actual.LoadBalancerName = lb.LoadBalancerName
	actual.DNSName = lb.DNSName
	actual.HostedZoneId = lb.CanonicalHostedZoneId //CanonicalHostedZoneNameID
	actual.Scheme = lb.Scheme
	actual.LoadBalancerArn = loadBalancerArn
	actual.VPC = &VPC{ID: lb.VpcId}
	actual.Type = lb.Type

	//do we want the rest of the items that are not one to one mapping w/ the aws api? ie. listenerArns?

	tagMap, err := describeLoadBalancerTags(cloud, []string{*loadBalancerArn})
	if err != nil {
		return nil, err
	}
	actual.Tags = make(map[string]string)
	for _, tag := range tagMap[*loadBalancerArn] {
		actual.Tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}

	for _, az := range lb.AvailabilityZones {
		actual.Subnets = append(actual.Subnets, &Subnet{ID: az.SubnetId})
	}

	/*for _, sg := range lb.SecurityGroups {
		actual.SecurityGroups = append(actual.SecurityGroups, &SecurityGroup{ID: sg})
	}*/

	{
		//TODO: discuss if necessary to use describeTargetGroupsPages ? right now we are hard coding the limitation to one target group for the nlb. is that fine?
		//other option, query for all target groups use pagination, and search for the tags or use a special name for the targetGroup and just search by that name. (probably easiset solution)
		request := &elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: loadBalancerArn,
		}
		response, err := cloud.ELBV2().DescribeTargetGroups(request)
		if err != nil {
			return nil, fmt.Errorf("error querying for NLB Target groups :%v", err)
		}

		if len(response.TargetGroups) == 0 {
			return nil, fmt.Errorf("Found no Target Groups for NLB don't think this is a normal condition :  %q", loadBalancerArn)
		}

		if len(response.TargetGroups) != 1 {
			return nil, fmt.Errorf("Found multiple Target groups for NLB with arn %q", loadBalancerArn)
		}

		targetGroupArn = response.TargetGroups[0].TargetGroupArn
		actual.TargetGroupArn = targetGroupArn

	}

	{
		//LoadBalancerArn
		request := &elbv2.DescribeListenersInput{
			LoadBalancerArn: loadBalancerArn,
		}
		response, err := cloud.ELBV2().DescribeListeners(request)
		if err != nil {
			return nil, fmt.Errorf("error querying for NLB listeners :%v", err)
		}

		actual.Listeners = make(map[string]*LoadBalancerListener)

		for _, l := range response.Listeners {
			loadBalancerPort := strconv.FormatInt(aws.Int64Value(l.Port), 10)

			actualListener := &LoadBalancerListener{}
			actualListener.InstancePort = int(aws.Int64Value(l.Port))
			//actualListener.SSLCertificateID = aws.StringValue(l.SSLCertificateId)    //TODO [HTTPS or TLS listener] The default certificate for the listener. Certificates []*Certificate `type:"list"`
			actual.Listeners[loadBalancerPort] = actualListener

			actual.ListenerArns = append(actual.ListenerArns, l.ListenerArn) //TODO: move ListenerArn to LoadBalancerListener
		}

	}

	healthcheck, err := findHealthCheck(cloud, lb)
	if err != nil {
		return nil, err
	}
	actual.HealthCheck = healthcheck

	// TODO: Extract attributes
	{
		lbAttributes, err := findLoadBalancerAttributes(cloud, aws.StringValue(loadBalancerArn))
		if err != nil {
			return nil, err
		}
		klog.V(4).Infof("NLB Load Balancer attributes: %+v", lbAttributes)

		actual.AccessLog = &LoadBalancerAccessLog{}
		actual.DeletionProtection = &LoadBalancerDeletionProtection{}
		actual.CrossZoneLoadBalancing = &LoadBalancerCrossZoneLoadBalancing{}
		for _, attribute := range lbAttributes {
			if attribute.Value == nil { //TODO: what if a value is nil? do we just leave it? something like...
				continue
			}
			switch key, value := attribute.Key, attribute.Value; *key {
			case "access_logs.s3.enabled":
				b, _ := strconv.ParseBool(*value) // TODO: check for error
				actual.AccessLog.Enabled = fi.Bool(b)
			case "access_logs.s3.bucket":
				actual.AccessLog.S3BucketName = value
			case "access_logs.s3.prefix":
				actual.AccessLog.S3BucketPrefix = value
			case "deletion_protection.enabled":
				b, _ := strconv.ParseBool(*value) // TODO: check for error
				actual.DeletionProtection.Enabled = fi.Bool(b)
			case "load_balancing.cross_zone.enabled":
				b, _ := strconv.ParseBool(*value) // TODO: check for error
				actual.CrossZoneLoadBalancing.Enabled = fi.Bool(b)
			default:
				fmt.Printf("unsupported key -- ignoring.\n") // TODO: Return error?
			}
		}
	}

	{
		tgAttributes, err := findTargetGroupAttributes(cloud, aws.StringValue(targetGroupArn))
		if err != nil {
			return nil, err
		}
		klog.V(4).Infof("Target Group attributes: %+v", tgAttributes)

		actual.ProxyProtocolV2 = &TargetGroupProxyProtocolV2{}
		actual.Stickiness = &TargetGroupStickiness{}
		actual.DeregistationDelay = &TargetGroupDeregistrationDelay{}
		for _, attribute := range tgAttributes {
			if attribute.Value == nil { //TODO: what if a value is nil? do we just leave it? something like...
				continue
			}
			switch key, value := attribute.Key, attribute.Value; *key {
			case "proxy_protocol_v2.enabled":
				b, _ := strconv.ParseBool(*value) // TODO: check for error
				actual.ProxyProtocolV2.Enabled = fi.Bool(b)
			case "stickiness.type":
				actual.Stickiness.Type = value
			case "stickiness.enabled":
				b, _ := strconv.ParseBool(*value) // TODO: check for error
				actual.Stickiness.Enabled = fi.Bool(b)
			case "deregistration_delay.timeout_seconds":
				if n, err := strconv.Atoi(*value); err == nil {
					m := int64(n)
					actual.DeregistationDelay.TimeoutSeconds = fi.Int64(m)
				} else {
					fmt.Println(s, "is not an integer.") //TODO: check for error
				}

			default:
				fmt.Printf("unsupported key -- ignoring.\n") //TODO: return error?
			}
		}
	}

	/*if lbAttributes != nil {
		actual.AccessLog = &LoadBalancerAccessLog{}
		if lbAttributes.AccessLog.EmitInterval != nil {
			actual.AccessLog.EmitInterval = lbAttributes.AccessLog.EmitInterval
		}
		if lbAttributes.AccessLog.Enabled != nil {
			actual.AccessLog.Enabled = lbAttributes.AccessLog.Enabled
		}
		if lbAttributes.AccessLog.S3BucketName != nil {
			actual.AccessLog.S3BucketName = lbAttributes.AccessLog.S3BucketName
		}
		if lbAttributes.AccessLog.S3BucketPrefix != nil {
			actual.AccessLog.S3BucketPrefix = lbAttributes.AccessLog.S3BucketPrefix
		}

		actual.ConnectionDraining = &LoadBalancerConnectionDraining{}
		if lbAttributes.ConnectionDraining.Enabled != nil {
			actual.ConnectionDraining.Enabled = lbAttributes.ConnectionDraining.Enabled
		}
		if lbAttributes.ConnectionDraining.Timeout != nil {
			actual.ConnectionDraining.Timeout = lbAttributes.ConnectionDraining.Timeout
		}

		actual.ConnectionSettings = &LoadBalancerConnectionSettings{}
		if lbAttributes.ConnectionSettings.IdleTimeout != nil {
			actual.ConnectionSettings.IdleTimeout = lbAttributes.ConnectionSettings.IdleTimeout
		}

		actual.CrossZoneLoadBalancing = &LoadBalancerCrossZoneLoadBalancing{}
		if lbAttributes.CrossZoneLoadBalancing.Enabled != nil {
			actual.CrossZoneLoadBalancing.Enabled = lbAttributes.CrossZoneLoadBalancing.Enabled
		}
	}*/

	// Avoid spurious mismatches
	if subnetSlicesEqualIgnoreOrder(actual.Subnets, e.Subnets) {
		actual.Subnets = e.Subnets
	}
	if e.DNSName == nil {
		e.DNSName = actual.DNSName
	}
	if e.HostedZoneId == nil {
		e.HostedZoneId = actual.HostedZoneId
	}
	if e.LoadBalancerName == nil {
		e.LoadBalancerName = actual.LoadBalancerName
	}

	actual.AgNames = e.AgNames // just to avoid spurious mismatches Need to get rid of this
	actual.UseNLBForAPI = e.UseNLBForAPI

	// We allow for the LoadBalancerName to be wrong:
	// 1. We don't want to force a rename of the ELB, because that is a destructive operation
	// 2. We were creating ELBs with insufficiently qualified names previously
	if fi.StringValue(e.LoadBalancerName) != fi.StringValue(actual.LoadBalancerName) {
		klog.V(2).Infof("Reusing existing load balancer with name: %q", aws.StringValue(actual.LoadBalancerName))
		e.LoadBalancerName = actual.LoadBalancerName
	}

	// TODO: Make Normalize a standard method
	actual.Normalize()

	klog.V(4).Infof("Found NLB %+v", actual)

	return actual, nil
}

var _ fi.HasAddress = &LoadBalancer{}

func (e *LoadBalancer) FindIPAddress(context *fi.Context) (*string, error) {
	fmt.Println("**** FindIPAddress")
	cloud := context.Cloud.(awsup.AWSCloud)

	lb, err := FindLoadBalancerByNameTag(cloud, fi.StringValue(e.Name))
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, nil
	}
	fmt.Println("findIPAddress should not arrive here unless --yes")

	lbDnsName := fi.StringValue(lb.DNSName)
	if lbDnsName == "" {
		return nil, nil
	}
	return &lbDnsName, nil
}

func (e *LoadBalancer) Run(c *fi.Context) error {
	fmt.Println("**** Run")
	// TODO: Make Normalize a standard method
	e.Normalize()

	return fi.DefaultDeltaRunMethod(e, c)
}

func (e *LoadBalancer) Normalize() {
	fmt.Println("**** Normalize")
	// We need to sort our arrays consistently, so we don't get spurious changes
	sort.Stable(OrderSubnetsById(e.Subnets))
	sort.Stable(OrderSecurityGroupsById(e.SecurityGroups))
}

func (s *LoadBalancer) CheckChanges(a, e, changes *LoadBalancer) error {
	fmt.Println("**** CheckChanges")
	if a == nil {
		if fi.StringValue(e.Name) == "" {
			return fi.RequiredField("Name")
		}
		// if len(e.SecurityGroups) == 0 {
		// 	return fi.RequiredField("SecurityGroups")
		// }
		if len(e.Subnets) == 0 {
			return fi.RequiredField("Subnets")
		}

		/*if e.AccessLog != nil {
			if e.AccessLog.Enabled == nil {
				return fi.RequiredField("Acceslog.Enabled")
			}
			if *e.AccessLog.Enabled {
				if e.AccessLog.S3BucketName == nil {
					return fi.RequiredField("Acceslog.S3Bucket")
				}
			}
		}
		if e.ConnectionDraining != nil {
			if e.ConnectionDraining.Enabled == nil {
				return fi.RequiredField("ConnectionDraining.Enabled")
			}
		}*/

		if e.CrossZoneLoadBalancing != nil {
			if e.CrossZoneLoadBalancing.Enabled == nil {
				return fi.RequiredField("CrossZoneLoadBalancing.Enabled")
			}
		}
	}

	return nil
}

func (_ *LoadBalancer) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer) error {
	fmt.Println("**** RenderAWS-christian")
	var loadBalancerName string
	var loadBalancerArn string

	if a == nil {
		if e.LoadBalancerName == nil {
			return fi.RequiredField("LoadBalancerName")
		}
		loadBalancerName = *e.LoadBalancerName

		request := &elbv2.CreateLoadBalancerInput{}
		request.Name = e.LoadBalancerName
		request.Scheme = e.Scheme
		request.Type = e.Type

		for _, subnet := range e.Subnets {
			request.Subnets = append(request.Subnets, subnet.ID)
		}

		//request.SecurityGroups = append(request.SecurityGroups, sg.ID)

		/*for _, sg := range e.SecurityGroups {
			request.SecurityGroups = append(request.SecurityGroups, sg.ID)
		}*/

		{
			klog.V(2).Infof("Creating NLB with Name:%q", loadBalancerName)

			response, err := t.Cloud.ELBV2().CreateLoadBalancer(request)
			if err != nil {
				return fmt.Errorf("error creating NLB: %v", err)
			}

			if len(response.LoadBalancers) != 1 {
				return fmt.Errorf("Either too many or too little NBLs were created, wanted to find %q", loadBalancerName)
			} else {
				loadBalancer := response.LoadBalancers[0] //TODO: how to avoid doing this
				e.DNSName = loadBalancer.DNSName
				e.HostedZoneId = loadBalancer.CanonicalHostedZoneId
				e.LoadBalancerArn = loadBalancer.LoadBalancerArn
				loadBalancerArn = fi.StringValue(loadBalancer.LoadBalancerArn) //todo; should i use a local variable ? where can i read more about this
			}

			// TODO: temporarily putting this here as i am tired of manually deleting the nlb on failed creations
			if err := t.AddELBV2Tags(loadBalancerArn, e.Tags); err != nil {
				return err
			}
		}

		{
			first15Char := loadBalancerName[:15]
			targetGroupName := first15Char + "-targets"
			//TODO: GET 443/TCP FROM e.loadbalancer
			request := &elbv2.CreateTargetGroupInput{
				Name:     aws.String(targetGroupName),
				Port:     aws.Int64(443),
				Protocol: aws.String("TCP"),
				VpcId:    e.VPC.ID,
			}

			fmt.Println("Creating Target Group for NLB")
			response, err := t.Cloud.ELBV2().CreateTargetGroup(request)
			if err != nil {
				return fmt.Errorf("Error creating target group for NLB : %v", err)
			}

			e.TargetGroupArn = response.TargetGroups[0].TargetGroupArn

			if err := t.AddELBV2Tags(*e.TargetGroupArn, e.Tags); err != nil {
				return err
			}
		}

		/// DEBUG CODE TO SEE WHAT WE GET BACK
		{
			{
				lbAttributes, err := findLoadBalancerAttributes(t.Cloud, loadBalancerArn)
				if err != nil {
					return err
				}
				klog.V(4).Infof("NLB Load Balancer attributes: %+v", lbAttributes)
				fmt.Printf("NLB Load Balancer attributes: %+v", lbAttributes)

				for _, attribute := range lbAttributes {
					if attribute.Value == nil { //TODO: what if a value is nil? do we just leave it? something like...
						fmt.Printf("%+v is empty\n", *attribute.Key)
						continue
					}
					key, val := *attribute.Key, *attribute.Value
					fmt.Printf("%+v : %+v\n", key, val)
				}
			}

			{
				tgAttributes, err := findTargetGroupAttributes(t.Cloud, aws.StringValue(e.TargetGroupArn))
				if err != nil {
					return err
				}
				klog.V(4).Infof("Target Group attributes: %+v", tgAttributes)
				fmt.Printf("Target Group attributes: %+v", tgAttributes)

				for _, attribute := range tgAttributes {
					if attribute.Value == nil { //TODO: what if a value is nil? do we just leave it? something like...
						fmt.Printf("%+v is empty\n", *attribute.Key)
						continue
					}
					key, val := *attribute.Key, *attribute.Value
					fmt.Printf("%+v : %+v\n", key, val)
				}
			}
		}

		{
			for loadBalancerPort, _ := range e.Listeners {
				loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
				if err != nil {
					return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
				}
				//TODO: how to deal w/ the SSL certificate?
				//awsListener := listener.mapToAWS2(loadBalancerPortInt)

				request := &elbv2.CreateListenerInput{
					DefaultActions: []*elbv2.Action{
						{
							TargetGroupArn: e.TargetGroupArn,
							Type:           aws.String("forward"),
						},
					},
					LoadBalancerArn: aws.String(loadBalancerArn),
					Protocol:        aws.String("TCP"),
				}
				request.SetPort(loadBalancerPortInt)

				fmt.Println("Creating Listener for NLB")
				response, err := t.Cloud.ELBV2().CreateListener(request)
				if err != nil {
					return fmt.Errorf("Error creating listener for NLB: %v", err)
				}
				e.ListenerArns = append(e.ListenerArns, response.Listeners[0].ListenerArn)
			}
		}
	} else {
		loadBalancerName = fi.StringValue(a.LoadBalancerName)
		loadBalancerArn = fi.StringValue(a.LoadBalancerArn)

		if changes.Subnets != nil {
			var expectedSubnets []string
			for _, s := range e.Subnets {
				expectedSubnets = append(expectedSubnets, fi.StringValue(s.ID))
			}

			var actualSubnets []string
			for _, s := range a.Subnets {
				actualSubnets = append(actualSubnets, fi.StringValue(s.ID))
			}

			oldSubnetIDs := slice.GetUniqueStrings(expectedSubnets, actualSubnets)
			if len(oldSubnetIDs) > 0 {
				/*request := &elb.DetachLoadBalancerFromSubnetsInput{}
				request.SetLoadBalancerName(loadBalancerName)
				request.SetSubnets(aws.StringSlice(oldSubnetIDs))

				klog.V(2).Infof("Detaching Load Balancer from old subnets")
				if _, err := t.Cloud.ELB().DetachLoadBalancerFromSubnets(request); err != nil {
					return fmt.Errorf("Error detaching Load Balancer from old subnets: %v", err)
				}*/
				return fmt.Errorf("Error, NLB's don't support detatching subnets, peraps we need to recreate the NLB")
			}

			newSubnetIDs := slice.GetUniqueStrings(actualSubnets, expectedSubnets)
			if len(newSubnetIDs) > 0 {

				request := &elbv2.SetSubnetsInput{}
				request.SetLoadBalancerArn(loadBalancerArn)
				request.SetSubnets(aws.StringSlice(append(actualSubnets, newSubnetIDs...)))

				klog.V(2).Infof("Attaching Load Balancer to new subnets")
				if _, err := t.Cloud.ELBV2().SetSubnets(request); err != nil {
					return fmt.Errorf("Error attaching Load Balancer to new subnets: %v", err)
				}
			}
		}

		//TODO: do something about security groups
		/*if changes.SecurityGroups != nil {
			request := &elb.ApplySecurityGroupsToLoadBalancerInput{}
			request.LoadBalancerName = aws.String(loadBalancerName)
			for _, sg := range e.SecurityGroups {
				request.SecurityGroups = append(request.SecurityGroups, sg.ID)
			}

			klog.V(2).Infof("Updating Load Balancer Security Groups")
			if _, err := t.Cloud.ELB().ApplySecurityGroupsToLoadBalancer(request); err != nil {
				return fmt.Errorf("Error updating security groups on Load Balancer: %v", err)
			}
		}*/

		if changes.Listeners != nil {

			elbDescription, err := findLoadBalancerByLoadBalancerName2(t.Cloud, loadBalancerName)
			if err != nil {
				return fmt.Errorf("error getting load balancer by name: %v", err)
			}

			if elbDescription != nil {
				for _, listenerArn := range e.ListenerArns {
					// deleting the listener before recreating it
					t.Cloud.ELBV2().DeleteListener(&elbv2.DeleteListenerInput{
						ListenerArn: listenerArn,
					})
					if err != nil {
						return fmt.Errorf("error deleting load balancer listener with arn = : %q : %v", listenerArn, err)
					}
				}
			}

			for loadBalancerPort, _ := range changes.Listeners {
				loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
				if err != nil {
					return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
				}
				//TODO: how to deal w/ the SSL certificate?
				//awsListener := listener.mapToAWS2(loadBalancerPortInt)

				request := &elbv2.CreateListenerInput{
					DefaultActions: []*elbv2.Action{
						{
							TargetGroupArn: e.TargetGroupArn,
							Type:           aws.String("forward"),
						},
					},
					LoadBalancerArn: aws.String(loadBalancerArn),
					Protocol:        aws.String("TCP"),
				}
				request.SetPort(loadBalancerPortInt)

				fmt.Println("Creating Listener for NLB")
				response, err := t.Cloud.ELBV2().CreateListener(request)
				if err != nil {
					return fmt.Errorf("Error creating listener for NLB: %v", err)
				}
				a.ListenerArns = append(e.ListenerArns, response.Listeners[0].ListenerArn) //or should this be changes?
			}
		}
	}

	//ok so by this point we have an nlb we probably need to tag it.
	//pickup here after lunch. go eat. nice work.

	if err := t.AddELBV2Tags(loadBalancerArn, e.Tags); err != nil {
		return err
	}

	/*if err := t.RemoveELBV2Tags(loadBalancerArn, e.Tags); err != nil {
		return err
	}*/

	if changes.HealthCheck != nil && e.HealthCheck != nil {
		//TODO: either split e.HealthCheck.Target on : or modify data structure to use Port / Protocol
		//NOTE:  With Network Load Balancers, you can't modify this setting, can only be TCP
		request := &elbv2.ModifyTargetGroupInput{
			HealthCheckPort: e.HealthCheck.Port,
			//HealthCheckProtocol:        e.HealthCheck.Protocol, //TODO: make sure this is not a settable option for nlb: // With Network Load Balancers, you can't modify this setting.
			TargetGroupArn: e.TargetGroupArn,
			//HealthCheckIntervalSeconds: e.HealthCheck.Interval, //TODO: make sure this is not a settable option for nlb: // With Network Load Balancers, you can't modify this setting.
			HealthyThresholdCount:   e.HealthCheck.HealthyThreshold,
			UnhealthyThresholdCount: e.HealthCheck.UnhealthyThreshold,
			//HealthCheckTimeoutSeconds:  e.HealthCheck.Timeout, //TODO: make sure this is not a settable option for nlbL // With Network Load Balancers, you can't modify this setting.
		}

		fmt.Printf("Configuring health checks on NLB %q", loadBalancerName)
		klog.V(2).Infof("Configuring health checks on NLB %q", loadBalancerName)
		_, err := t.Cloud.ELBV2().ModifyTargetGroup(request)
		if err != nil {
			return fmt.Errorf("error configuring health checks on NLB: %v's target group", err)
		}
	}

	if err := e.modifyLoadBalancerAttributes(t, a, e, changes); err != nil {
		klog.Infof("error modifying NLB attributes: %v", err)
		return err
	}

	if err := e.modifyTargetGroupAttributes(t, a, e, changes); err != nil {
		klog.Infof("error modifying NLB Target Group attributes: %v", err)
		return err
	}

	return nil
}

/*func (_ *LoadBalancer) RenderAWSOld(t *awsup.AWSAPITarget, a, e, changes *LoadBalancer) error {
	fmt.Println("**** RenderAWS")
	var loadBalancerName string
	if a == nil {
		if e.LoadBalancerName == nil {
			return fi.RequiredField("LoadBalancerName")
		}
		loadBalancerName = *e.LoadBalancerName

		request := &elb.CreateLoadBalancerInput{}
		request.LoadBalancerName = e.LoadBalancerName
		request.Scheme = e.Scheme

		for _, subnet := range e.Subnets {
			request.Subnets = append(request.Subnets, subnet.ID)
		}

		for _, sg := range e.SecurityGroups {
			request.SecurityGroups = append(request.SecurityGroups, sg.ID)
		}

		request.Listeners = []*elb.Listener{}

		for loadBalancerPort, listener := range e.Listeners {
			loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
			if err != nil {
				return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
			}
			awsListener := listener.mapToAWS(loadBalancerPortInt)
			request.Listeners = append(request.Listeners, awsListener)
		}

		klog.V(2).Infof("Creating ELB with Name:%q", loadBalancerName)

		response, err := t.Cloud.ELB().CreateLoadBalancer(request)
		if err != nil {
			return fmt.Errorf("error creating ELB: %v", err)
		}

		e.DNSName = response.DNSName

		// Requery to get the CanonicalHostedZoneNameID
		lb, err := findLoadBalancerByLoadBalancerName(t.Cloud, loadBalancerName)
		if err != nil {
			return err
		}
		if lb == nil {
			// TODO: Retry?  Is this async
			return fmt.Errorf("Unable to find newly created ELB %q", loadBalancerName)
		}
		e.HostedZoneId = lb.CanonicalHostedZoneNameID
	} else {
		loadBalancerName = fi.StringValue(a.LoadBalancerName)

		if changes.Subnets != nil {
			var expectedSubnets []string
			for _, s := range e.Subnets {
				expectedSubnets = append(expectedSubnets, fi.StringValue(s.ID))
			}

			var actualSubnets []string
			for _, s := range a.Subnets {
				actualSubnets = append(actualSubnets, fi.StringValue(s.ID))
			}

			oldSubnetIDs := slice.GetUniqueStrings(expectedSubnets, actualSubnets)
			if len(oldSubnetIDs) > 0 {
				request := &elb.DetachLoadBalancerFromSubnetsInput{}
				request.SetLoadBalancerName(loadBalancerName)
				request.SetSubnets(aws.StringSlice(oldSubnetIDs))

				klog.V(2).Infof("Detaching Load Balancer from old subnets")
				if _, err := t.Cloud.ELB().DetachLoadBalancerFromSubnets(request); err != nil {
					return fmt.Errorf("Error detaching Load Balancer from old subnets: %v", err)
				}
			}

			newSubnetIDs := slice.GetUniqueStrings(actualSubnets, expectedSubnets)
			if len(newSubnetIDs) > 0 {
				request := &elb.AttachLoadBalancerToSubnetsInput{}
				request.SetLoadBalancerName(loadBalancerName)
				request.SetSubnets(aws.StringSlice(newSubnetIDs))

				klog.V(2).Infof("Attaching Load Balancer to new subnets")
				if _, err := t.Cloud.ELB().AttachLoadBalancerToSubnets(request); err != nil {
					return fmt.Errorf("Error attaching Load Balancer to new subnets: %v", err)
				}
			}
		}

		if changes.SecurityGroups != nil {
			request := &elb.ApplySecurityGroupsToLoadBalancerInput{}
			request.LoadBalancerName = aws.String(loadBalancerName)
			for _, sg := range e.SecurityGroups {
				request.SecurityGroups = append(request.SecurityGroups, sg.ID)
			}

			klog.V(2).Infof("Updating Load Balancer Security Groups")
			if _, err := t.Cloud.ELB().ApplySecurityGroupsToLoadBalancer(request); err != nil {
				return fmt.Errorf("Error updating security groups on Load Balancer: %v", err)
			}
		}

		if changes.Listeners != nil {

			elbDescription, err := findLoadBalancerByLoadBalancerName(t.Cloud, loadBalancerName)
			if err != nil {
				return fmt.Errorf("error getting load balancer by name: %v", err)
			}

			if elbDescription != nil {
				// deleting the listener before recreating it
				t.Cloud.ELB().DeleteLoadBalancerListeners(&elb.DeleteLoadBalancerListenersInput{
					LoadBalancerName:  aws.String(loadBalancerName),
					LoadBalancerPorts: []*int64{aws.Int64(443)},
				})
			}

			request := &elb.CreateLoadBalancerListenersInput{}
			request.LoadBalancerName = aws.String(loadBalancerName)

			for loadBalancerPort, listener := range changes.Listeners {
				loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
				if err != nil {
					return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
				}
				awsListener := listener.mapToAWS(loadBalancerPortInt)
				request.Listeners = append(request.Listeners, awsListener)
			}

			klog.V(2).Infof("Creating LoadBalancer listeners")

			_, err = t.Cloud.ELB().CreateLoadBalancerListeners(request)
			if err != nil {
				return fmt.Errorf("error creating LoadBalancerListeners: %v", err)
			}
		}
	}

	if err := t.AddELBTags(loadBalancerName, e.Tags); err != nil {
		return err
	}

	if err := t.RemoveELBTags(loadBalancerName, e.Tags); err != nil {
		return err
	}

	if changes.HealthCheck != nil && e.HealthCheck != nil {
		request := &elb.ConfigureHealthCheckInput{}
		request.LoadBalancerName = aws.String(loadBalancerName)
		request.HealthCheck = &elb.HealthCheck{
			Target:             e.HealthCheck.Target,
			HealthyThreshold:   e.HealthCheck.HealthyThreshold,
			UnhealthyThreshold: e.HealthCheck.UnhealthyThreshold,
			Interval:           e.HealthCheck.Interval,
			Timeout:            e.HealthCheck.Timeout,
		}

		klog.V(2).Infof("Configuring health checks on ELB %q", loadBalancerName)

		_, err := t.Cloud.ELB().ConfigureHealthCheck(request)
		if err != nil {
			return fmt.Errorf("error configuring health checks on ELB: %v", err)
		}
	}

	if err := e.modifyLoadBalancerAttributes(t, a, e, changes); err != nil {
		klog.Infof("error modifying ELB attributes: %v", err)
		return err
	}

	return nil
}*/

type terraformLoadBalancer struct {
	LoadBalancerName *string                          `json:"name"`
	Listener         []*terraformLoadBalancerListener `json:"listener"`
	SecurityGroups   []*terraform.Literal             `json:"security_groups"`
	Subnets          []*terraform.Literal             `json:"subnets"`
	Internal         *bool                            `json:"internal,omitempty"`

	HealthCheck *terraformLoadBalancerHealthCheck `json:"health_check,omitempty"`
	AccessLog   *terraformLoadBalancerAccessLog   `json:"access_logs,omitempty"`

	ConnectionDraining        *bool  `json:"connection_draining,omitempty"`
	ConnectionDrainingTimeout *int64 `json:"connection_draining_timeout,omitempty"`

	CrossZoneLoadBalancing *bool `json:"cross_zone_load_balancing,omitempty"`

	IdleTimeout *int64 `json:"idle_timeout,omitempty"`

	Tags map[string]string `json:"tags,omitempty"`
}

type terraformLoadBalancerListener struct {
	InstancePort     int    `json:"instance_port"`
	InstanceProtocol string `json:"instance_protocol"`
	LBPort           int64  `json:"lb_port"`
	LBProtocol       string `json:"lb_protocol"`
	SSLCertificateID string `json:"ssl_certificate_id,omitempty"`
}

type terraformLoadBalancerHealthCheck struct {
	Target             *string `json:"target"`
	HealthyThreshold   *int64  `json:"healthy_threshold"`
	UnhealthyThreshold *int64  `json:"unhealthy_threshold"`
	Interval           *int64  `json:"interval"`
	Timeout            *int64  `json:"timeout"`
}

func (_ *LoadBalancer) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *LoadBalancer) error {
	fmt.Println("**** ")
	cloud := t.Cloud.(awsup.AWSCloud)

	if e.LoadBalancerName == nil {
		return fi.RequiredField("LoadBalancerName")
	}

	tf := &terraformLoadBalancer{
		LoadBalancerName: e.LoadBalancerName,
	}
	if fi.StringValue(e.Scheme) == "internal" {
		tf.Internal = fi.Bool(true)
	}

	for _, subnet := range e.Subnets {
		tf.Subnets = append(tf.Subnets, subnet.TerraformLink())
	}
	terraform.SortLiterals(tf.Subnets)

	for _, sg := range e.SecurityGroups {
		tf.SecurityGroups = append(tf.SecurityGroups, sg.TerraformLink())
	}
	terraform.SortLiterals(tf.SecurityGroups)

	for loadBalancerPort, listener := range e.Listeners {
		loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
		if err != nil {
			return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
		}

		if listener.SSLCertificateID != "" {
			tf.Listener = append(tf.Listener, &terraformLoadBalancerListener{
				InstanceProtocol: "SSL",
				InstancePort:     listener.InstancePort,
				LBPort:           loadBalancerPortInt,
				LBProtocol:       "SSL",
				SSLCertificateID: listener.SSLCertificateID,
			})
		} else {
			tf.Listener = append(tf.Listener, &terraformLoadBalancerListener{
				InstanceProtocol: "TCP",
				InstancePort:     listener.InstancePort,
				LBPort:           loadBalancerPortInt,
				LBProtocol:       "TCP",
			})
		}

	}

	if e.HealthCheck != nil {
		tf.HealthCheck = &terraformLoadBalancerHealthCheck{
			Target:             e.HealthCheck.Target,
			HealthyThreshold:   e.HealthCheck.HealthyThreshold,
			UnhealthyThreshold: e.HealthCheck.UnhealthyThreshold,
			Interval:           e.HealthCheck.Interval,
			Timeout:            e.HealthCheck.Timeout,
		}
	}

	if e.AccessLog != nil {
		tf.AccessLog = &terraformLoadBalancerAccessLog{
			EmitInterval:   e.AccessLog.EmitInterval,
			Enabled:        e.AccessLog.Enabled,
			S3BucketName:   e.AccessLog.S3BucketName,
			S3BucketPrefix: e.AccessLog.S3BucketPrefix,
		}
	}

	if e.ConnectionDraining != nil {
		tf.ConnectionDraining = e.ConnectionDraining.Enabled
		tf.ConnectionDrainingTimeout = e.ConnectionDraining.Timeout
	}

	if e.ConnectionSettings != nil {
		tf.IdleTimeout = e.ConnectionSettings.IdleTimeout
	}

	if e.CrossZoneLoadBalancing != nil {
		tf.CrossZoneLoadBalancing = e.CrossZoneLoadBalancing.Enabled
	}

	var tags map[string]string = cloud.BuildTags(e.Name)
	for k, v := range e.Tags {
		tags[k] = v
	}
	tf.Tags = tags

	return t.RenderResource("aws_elb", *e.Name, tf)
}

func (e *LoadBalancer) TerraformLink(params ...string) *terraform.Literal {
	fmt.Println("**** ")
	prop := "id"
	if len(params) > 0 {
		prop = params[0]
	}
	return terraform.LiteralProperty("aws_elb", *e.Name, prop)
}

type cloudformationLoadBalancer struct {
	LoadBalancerName *string                               `json:"LoadBalancerName,omitempty"`
	Listener         []*cloudformationLoadBalancerListener `json:"Listeners,omitempty"`
	SecurityGroups   []*cloudformation.Literal             `json:"SecurityGroups,omitempty"`
	Subnets          []*cloudformation.Literal             `json:"Subnets,omitempty"`
	Scheme           *string                               `json:"Scheme,omitempty"`

	HealthCheck *cloudformationLoadBalancerHealthCheck `json:"HealthCheck,omitempty"`
	AccessLog   *cloudformationLoadBalancerAccessLog   `json:"AccessLoggingPolicy,omitempty"`

	ConnectionDrainingPolicy *cloudformationConnectionDrainingPolicy `json:"ConnectionDrainingPolicy,omitempty"`
	ConnectionSettings       *cloudformationConnectionSettings       `json:"ConnectionSettings,omitempty"`

	CrossZoneLoadBalancing *bool `json:"CrossZone,omitempty"`

	Tags []cloudformationTag `json:"Tags,omitempty"`
}

type cloudformationLoadBalancerListener struct {
	InstancePort         int    `json:"InstancePort"`
	InstanceProtocol     string `json:"InstanceProtocol"`
	LoadBalancerPort     int64  `json:"LoadBalancerPort"`
	LoadBalancerProtocol string `json:"Protocol"`
}

type cloudformationLoadBalancerHealthCheck struct {
	Target             *string `json:"Target"`
	HealthyThreshold   *int64  `json:"HealthyThreshold"`
	UnhealthyThreshold *int64  `json:"UnhealthyThreshold"`
	Interval           *int64  `json:"Interval"`
	Timeout            *int64  `json:"Timeout"`
}

type cloudformationConnectionDrainingPolicy struct {
	Enabled *bool  `json:"Enabled,omitempty"`
	Timeout *int64 `json:"Timeout,omitempty"`
}

type cloudformationConnectionSettings struct {
	IdleTimeout *int64 `json:"IdleTimeout,omitempty"`
}

func (_ *LoadBalancer) RenderCloudformation(t *cloudformation.CloudformationTarget, a, e, changes *LoadBalancer) error {
	fmt.Println("**** ")
	// TODO: From http://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ec2-elb.html:
	// If this resource has a public IP address and is also in a VPC that is defined in the same template,
	// you must use the DependsOn attribute to declare a dependency on the VPC-gateway attachment.

	cloud := t.Cloud.(awsup.AWSCloud)

	if e.LoadBalancerName == nil {
		return fi.RequiredField("LoadBalancerName")
	}

	tf := &cloudformationLoadBalancer{
		LoadBalancerName: e.LoadBalancerName,
		Scheme:           e.Scheme,
	}

	for _, subnet := range e.Subnets {
		tf.Subnets = append(tf.Subnets, subnet.CloudformationLink())
	}

	for _, sg := range e.SecurityGroups {
		tf.SecurityGroups = append(tf.SecurityGroups, sg.CloudformationLink())
	}

	for loadBalancerPort, listener := range e.Listeners {
		loadBalancerPortInt, err := strconv.ParseInt(loadBalancerPort, 10, 64)
		if err != nil {
			return fmt.Errorf("error parsing load balancer listener port: %q", loadBalancerPort)
		}

		tf.Listener = append(tf.Listener, &cloudformationLoadBalancerListener{
			InstanceProtocol:     "TCP",
			InstancePort:         listener.InstancePort,
			LoadBalancerPort:     loadBalancerPortInt,
			LoadBalancerProtocol: "TCP",
		})
	}

	if e.HealthCheck != nil {
		tf.HealthCheck = &cloudformationLoadBalancerHealthCheck{
			Target:             e.HealthCheck.Target,
			HealthyThreshold:   e.HealthCheck.HealthyThreshold,
			UnhealthyThreshold: e.HealthCheck.UnhealthyThreshold,
			Interval:           e.HealthCheck.Interval,
			Timeout:            e.HealthCheck.Timeout,
		}
	}

	if e.AccessLog != nil {
		tf.AccessLog = &cloudformationLoadBalancerAccessLog{
			EmitInterval:   e.AccessLog.EmitInterval,
			Enabled:        e.AccessLog.Enabled,
			S3BucketName:   e.AccessLog.S3BucketName,
			S3BucketPrefix: e.AccessLog.S3BucketPrefix,
		}
	}

	if e.ConnectionDraining != nil {
		tf.ConnectionDrainingPolicy = &cloudformationConnectionDrainingPolicy{
			Enabled: e.ConnectionDraining.Enabled,
			Timeout: e.ConnectionDraining.Timeout,
		}
	}

	if e.ConnectionSettings != nil {
		tf.ConnectionSettings = &cloudformationConnectionSettings{
			IdleTimeout: e.ConnectionSettings.IdleTimeout,
		}
	}

	if e.CrossZoneLoadBalancing != nil {
		tf.CrossZoneLoadBalancing = e.CrossZoneLoadBalancing.Enabled
	}

	var tags map[string]string = cloud.BuildTags(e.Name)
	for k, v := range e.Tags {
		tags[k] = v
	}

	tf.Tags = buildCloudformationTags(tags)

	return t.RenderResource("AWS::ElasticLoadBalancing::LoadBalancer", *e.Name, tf)
}

func (e *LoadBalancer) CloudformationLink() *cloudformation.Literal {
	fmt.Println("**** ")
	return cloudformation.Ref("AWS::ElasticLoadBalancing::LoadBalancer", *e.Name)
}

func (e *LoadBalancer) CloudformationAttrCanonicalHostedZoneNameID() *cloudformation.Literal {
	fmt.Println("**** ")
	return cloudformation.GetAtt("AWS::ElasticLoadBalancing::LoadBalancer", *e.Name, "CanonicalHostedZoneNameID")
}

func (e *LoadBalancer) CloudformationAttrDNSName() *cloudformation.Literal {
	fmt.Println("**** ")
	return cloudformation.GetAtt("AWS::ElasticLoadBalancing::LoadBalancer", *e.Name, "DNSName")
}
