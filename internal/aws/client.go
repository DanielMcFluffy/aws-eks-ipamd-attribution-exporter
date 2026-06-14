package aws

import (
	"context"
	"net/netip"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/attribution"
)

type EC2API interface {
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
}

type Client struct {
	ec2 EC2API
}

func NewClient(ec2Client EC2API) *Client {
	return &Client{ec2: ec2Client}
}

func (c *Client) ListSubnets(ctx context.Context, vpcID string) ([]attribution.Subnet, error) {
	var subnets []attribution.Subnet
	input := &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{{
			Name:   aws.String("vpc-id"),
			Values: []string{vpcID},
		}},
	}

	paginator := ec2.NewDescribeSubnetsPaginator(c.ec2, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, subnet := range page.Subnets {
			subnets = append(subnets, attribution.Subnet{
				ID:               aws.ToString(subnet.SubnetId),
				Name:             tagValue(subnet.Tags, "Name"),
				AvailabilityZone: aws.ToString(subnet.AvailabilityZone),
			})
		}
	}

	sort.Slice(subnets, func(i, j int) bool { return subnets[i].ID < subnets[j].ID })
	return subnets, nil
}

func (c *Client) ListNetworkInterfaces(ctx context.Context, subnetIDs []string) ([]attribution.NetworkInterface, error) {
	if len(subnetIDs) == 0 {
		return nil, nil
	}

	var enis []attribution.NetworkInterface
	input := &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{{
			Name:   aws.String("subnet-id"),
			Values: subnetIDs,
		}},
	}

	paginator := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, eni := range page.NetworkInterfaces {
			privateIPs := make([]attribution.PrivateIP, 0, len(eni.PrivateIpAddresses))
			for _, privateIP := range eni.PrivateIpAddresses {
				addr, err := netip.ParseAddr(aws.ToString(privateIP.PrivateIpAddress))
				if err != nil {
					continue
				}
				privateIPs = append(privateIPs, attribution.PrivateIP{
					Address: addr,
					Primary: aws.ToBool(privateIP.Primary),
				})
			}
			enis = append(enis, attribution.NetworkInterface{
				ID:                   aws.ToString(eni.NetworkInterfaceId),
				Description:          aws.ToString(eni.Description),
				InterfaceType:        string(eni.InterfaceType),
				SubnetID:             aws.ToString(eni.SubnetId),
				AttachmentInstanceID: aws.ToString(eni.Attachment.InstanceId),
				PrivateIPs:           privateIPs,
			})
		}
	}

	sort.Slice(enis, func(i, j int) bool { return enis[i].ID < enis[j].ID })
	return enis, nil
}

func tagValue(tags []types.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}
