package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type AWSProvider struct {
	client *ec2.Client
	name   string
	region string
}

func NewAWSProvider(ctx context.Context, optFns ...func(*config.LoadOptions) error) (*AWSProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("AWS region is required")
	}
	return &AWSProvider{
		client: ec2.NewFromConfig(cfg),
		name:   "AWS",
		region: cfg.Region,
	}, nil
}

func (p *AWSProvider) Kind() ProviderKind { return ProviderAWS }
func (p *AWSProvider) Name() string       { return p.name }
func (p *AWSProvider) Info() ProviderInfo {
	return ProviderInfo{Kind: p.Kind(), Name: p.Name(), Region: p.region}
}

func (p *AWSProvider) ListInstances(ctx context.Context) ([]Instance, error) {
	var instances []Instance
	var nextToken *string
	for {
		result, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}

		for _, reservation := range result.Reservations {
			for _, inst := range reservation.Instances {
				instance := Instance{
					ID:           *inst.InstanceId,
					Provider:     ProviderAWS,
					Region:       "",
					InstanceType: string(inst.InstanceType),
					Status:       string(inst.State.Name),
					CPU:          instanceVCPU(inst.InstanceType),
					MemoryMB:     instanceMemoryMB(inst.InstanceType),
					CreatedAt:    *inst.LaunchTime,
					Tags:         ec2TagsToMap(inst.Tags),
				}
				if inst.PublicIpAddress != nil {
					instance.PublicIP = *inst.PublicIpAddress
				}
				if inst.PrivateIpAddress != nil {
					instance.PrivateIP = *inst.PrivateIpAddress
				}
				if inst.Placement != nil && inst.Placement.AvailabilityZone != nil {
					instance.Region = *inst.Placement.AvailabilityZone
				}
				instances = append(instances, instance)
			}
		}

		nextToken = result.NextToken
		if nextToken == nil {
			break
		}
	}
	return instances, nil
}

func (p *AWSProvider) CreateInstance(ctx context.Context, req CreateInstanceRequest) (*Instance, error) {
	if req.Region != p.region {
		return nil, fmt.Errorf("AWS provider is configured for region %q, not %q", p.region, req.Region)
	}
	input := &ec2.RunInstancesInput{
		ImageId:      &req.Image,
		InstanceType: types.InstanceType(req.InstanceType),
		MinCount:     int32Ptr(1),
		MaxCount:     int32Ptr(1),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeInstance,
			Tags:         mapToEC2Tags(req.Tags),
		}},
	}
	if req.SubnetID != "" {
		input.SubnetId = &req.SubnetID
	}
	if len(req.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = req.SecurityGroupIDs
	}
	if req.IAMInstanceProfile != "" {
		input.IamInstanceProfile = &types.IamInstanceProfileSpecification{Name: &req.IAMInstanceProfile}
	}
	if req.UserData != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(req.UserData))
		input.UserData = &encoded
	}
	if req.DiskGB > 0 {
		input.BlockDeviceMappings = []types.BlockDeviceMapping{{DeviceName: strPtr("/dev/sda1"), Ebs: &types.EbsBlockDevice{VolumeSize: int32Ptr(int32(req.DiskGB)), VolumeType: types.VolumeTypeGp3, DeleteOnTermination: ptrBool(true)}}}
	}
	if req.Name != "" {
		input.TagSpecifications[0].Tags = append(input.TagSpecifications[0].Tags, types.Tag{Key: strPtr("Name"), Value: &req.Name})
	}
	if len(req.SSHKeys) > 0 {
		input.KeyName = &req.SSHKeys[0]
	}

	result, err := p.client.RunInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("run instances: %w", err)
	}
	if len(result.Instances) == 0 {
		return nil, fmt.Errorf("no instances created")
	}

	inst := result.Instances[0]
	instance := &Instance{
		ID:           *inst.InstanceId,
		Name:         req.Name,
		Provider:     ProviderAWS,
		Region:       req.Region,
		InstanceType: req.InstanceType,
		Status:       string(inst.State.Name),
		CreatedAt:    time.Now().UTC(),
		Tags:         req.Tags,
	}
	if inst.PrivateIpAddress != nil {
		instance.PrivateIP = *inst.PrivateIpAddress
	}

	return instance, nil
}

func (p *AWSProvider) DeleteInstance(ctx context.Context, id string) error {
	_, err := p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{id},
	})
	return err
}

func (p *AWSProvider) GetInstance(ctx context.Context, id string) (*Instance, error) {
	result, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instance: %w", err)
	}
	for _, reservation := range result.Reservations {
		for _, inst := range reservation.Instances {
			instance := &Instance{
				ID:        *inst.InstanceId,
				Provider:  ProviderAWS,
				Status:    string(inst.State.Name),
				CreatedAt: *inst.LaunchTime,
			}
			if inst.PublicIpAddress != nil {
				instance.PublicIP = *inst.PublicIpAddress
			}
			if inst.PrivateIpAddress != nil {
				instance.PrivateIP = *inst.PrivateIpAddress
			}
			return instance, nil
		}
	}
	return nil, fmt.Errorf("instance %s not found", id)
}

func (p *AWSProvider) StartInstance(ctx context.Context, id string) error {
	_, err := p.client.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{id}})
	return err
}

func (p *AWSProvider) StopInstance(ctx context.Context, id string) error {
	_, err := p.client.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{id}})
	return err
}

func (p *AWSProvider) ListRegions(ctx context.Context) ([]string, error) {
	return []string{p.region}, nil
}

func (p *AWSProvider) ListInstanceTypes(ctx context.Context, region string) ([]string, error) {
	if region != "" && region != p.region {
		return nil, fmt.Errorf("AWS provider is configured for region %q, not %q", p.region, region)
	}
	var instanceTypes []string
	var nextToken *string
	for {
		result, err := p.client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe instance types: %w", err)
		}
		for _, t := range result.InstanceTypes {
			if t.InstanceType != "" {
				instanceTypes = append(instanceTypes, string(t.InstanceType))
			}
		}
		nextToken = result.NextToken
		if nextToken == nil {
			break
		}
	}
	return instanceTypes, nil
}

func ec2TagsToMap(tags []types.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, t := range tags {
		if t.Key != nil && t.Value != nil {
			result[*t.Key] = *t.Value
		}
	}
	return result
}

func mapToEC2Tags(tags map[string]string) []types.Tag {
	result := make([]types.Tag, 0, len(tags))
	for k, v := range tags {
		key, val := k, v
		result = append(result, types.Tag{Key: &key, Value: &val})
	}
	return result
}

func instanceVCPU(instanceType types.InstanceType) int {
	lookup := map[string]int{
		"t2.nano": 1, "t2.micro": 1, "t2.small": 1, "t2.medium": 2, "t2.large": 2,
		"t3.nano": 2, "t3.micro": 2, "t3.small": 2, "t3.medium": 2, "t3.large": 2,
		"m5.large": 2, "m5.xlarge": 4, "m5.2xlarge": 8, "m5.4xlarge": 16,
		"c5.large": 2, "c5.xlarge": 4, "c5.2xlarge": 8, "c5.4xlarge": 16,
	}
	if v, ok := lookup[string(instanceType)]; ok {
		return v
	}
	return 2
}

func instanceMemoryMB(instanceType types.InstanceType) int {
	lookup := map[string]int{
		"t2.nano": 512, "t2.micro": 1024, "t2.small": 2048, "t2.medium": 4096, "t2.large": 8192,
		"t3.nano": 512, "t3.micro": 1024, "t3.small": 2048, "t3.medium": 4096, "t3.large": 8192,
		"m5.large": 8192, "m5.xlarge": 16384, "m5.2xlarge": 32768, "m5.4xlarge": 65536,
		"c5.large": 4096, "c5.xlarge": 8192, "c5.2xlarge": 16384, "c5.4xlarge": 32768,
	}
	if v, ok := lookup[string(instanceType)]; ok {
		return v
	}
	return 4096
}

func int32Ptr(i int32) *int32 { return &i }
func strPtr(s string) *string { return &s }
func ptrBool(v bool) *bool    { return &v }
