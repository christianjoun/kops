/*
Copyright 2017 The Kubernetes Authors.

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

package commands

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/aws/aws-sdk-go/aws"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/resources/digitalocean"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/upup/pkg/fi/cloudup/aliup"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstack"
)

// CloudDiscoveryStatusStore implements status.Store by inspecting cloud objects.
// Likely temporary until we validate our status usage
type CloudDiscoveryStatusStore struct {
}

var _ kops.StatusStore = &CloudDiscoveryStatusStore{}

func isNil(v interface{}) bool {
	return v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil())
}

func (s *CloudDiscoveryStatusStore) GetApiIngressStatus(cluster *kops.Cluster) ([]kops.ApiIngressStatus, error) {
	cloud, err := cloudup.BuildCloud(cluster)
	if err != nil {
		return nil, err
	}

	if aliCloud, ok := cloud.(aliup.ALICloud); ok {
		return aliCloud.GetApiIngressStatus(cluster)
	}

	if gceCloud, ok := cloud.(gce.GCECloud); ok {
		return gceCloud.GetApiIngressStatus(cluster)
	}

	if awsCloud, ok := cloud.(awsup.AWSCloud); ok {
		name := "api." + cluster.Name

		ELB := true
		NLB := false
		var lb interface{}

		if ELB {
			lb, err = awstasks.FindLoadBalancerByNameTag(awsCloud, name)
			if isNil(lb) {
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("error looking for AWS ELB: %v", err)
			}
		} else if NLB {
			lb, err = awstasks.FindNetworkLoadBalancerByNameTag(awsCloud, name)
			if isNil(lb) {
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("error looking for AWS ELB: %v", err)
			}

		}
		var ingresses []kops.ApiIngressStatus

		if !isNil(lb) {
			var lbDnsName string
			if ELB {
				lbDnsName = aws.StringValue(lb.(*elb.LoadBalancerDescription).DNSName)
			} else if NLB {
				lbDnsName = aws.StringValue(lb.(*elbv2.LoadBalancer).DNSName)
			}
			//lbDnsName := aws.StringValue(lb.DNSName)
			if lbDnsName == "" {
				return nil, fmt.Errorf("found api LB %q, but it did not have a DNSName", name)
			}

			ingresses = append(ingresses, kops.ApiIngressStatus{Hostname: lbDnsName})
		}

		return ingresses, nil
	}

	if osCloud, ok := cloud.(openstack.OpenstackCloud); ok {
		return osCloud.GetApiIngressStatus(cluster)
	}

	if doCloud, ok := cloud.(*digitalocean.Cloud); ok {
		return doCloud.GetApiIngressStatus(cluster)
	}

	return nil, fmt.Errorf("API Ingress Status not implemented for %T", cloud)
}

// FindClusterStatus discovers the status of the cluster, by inspecting the cloud objects
func (s *CloudDiscoveryStatusStore) FindClusterStatus(cluster *kops.Cluster) (*kops.ClusterStatus, error) {
	cloud, err := cloudup.BuildCloud(cluster)
	if err != nil {
		return nil, err
	}

	if gceCloud, ok := cloud.(gce.GCECloud); ok {
		return gceCloud.FindClusterStatus(cluster)
	}

	if awsCloud, ok := cloud.(awsup.AWSCloud); ok {
		return awsCloud.FindClusterStatus(cluster)
	}

	if aliCloud, ok := cloud.(aliup.ALICloud); ok {
		return aliCloud.FindClusterStatus(cluster)
	}

	if osCloud, ok := cloud.(openstack.OpenstackCloud); ok {
		return osCloud.FindClusterStatus(cluster)
	}
	return nil, fmt.Errorf("etcd Status not implemented for %T", cloud)
}
