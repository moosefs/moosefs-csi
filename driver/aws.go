package driver

import (
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type AwsCreds struct {
	ID     string
	secret string
	token  string
}

type MfsType struct {
	name    string
	version string
}

// Maintains the AWS entities created during ECS creation
type ECSStore struct {
	Service       *ecs.CreateServiceOutput
	Task          *ecs.RegisterTaskDefinitionOutput
	SecurityGroup *ec2.CreateSecurityGroupOutput
}

// CreateECSCluster ...
func CreateECSCluster(creds AwsCreds, region, name string) (*ecs.CreateClusterOutput, error) {

	sess, err := session.NewSession(
		&aws.Config{Region: aws.String(region),
			Credentials: credentials.NewStaticCredentials(creds.ID, creds.secret, creds.token),
		})

	if err != nil {
		return nil, err
	}

	svc := ecs.New(sess)
	input := &ecs.CreateClusterInput{
		ClusterName: aws.String(name),
	}

	result, err := svc.CreateCluster(input)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteECSCluster ...
func DeleteECSCluster(creds AwsCreds, region, name string) (*ecs.DeleteClusterOutput, error) {
	sess, err := session.NewSession(
		&aws.Config{Region: aws.String(region),
			Credentials: credentials.NewStaticCredentials(creds.ID, creds.secret, creds.token),
		})
	if err != nil {
		return nil, err
	}

	svc := ecs.New(sess)
	input := &ecs.DeleteClusterInput{
		Cluster: aws.String(name),
	}

	result, err := svc.DeleteCluster(input)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// CreateECSService ...
func CreateECSService(creds AwsCreds, region, name, clusterName, taskName string, mfsType MfsType) (ECSStore, error) {
	store := ECSStore{}
	sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(creds.ID, creds.secret, creds.token),
		})
	if err != nil {
		return store, err
	}

	svc := ecs.New(sess)
	output, err := registerTaskDefinition(svc, taskName, mfsType)
	if err != nil {
		return store, err
	}
	store.Task = output

	sg, err := createSecurityGroup(clusterName, "Created for moosefs-csi-fargate", region)
	if err != nil {
		return store, err
	}
	store.SecurityGroup = sg
	gid := *sg.GroupId

	input := &ecs.CreateServiceInput{
		Cluster:        aws.String(clusterName),
		DesiredCount:   aws.Int64(1),
		ServiceName:    aws.String(name),
		TaskDefinition: aws.String(taskName),
		LaunchType:     aws.String("FARGATE"),
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String(ecs.AssignPublicIpEnabled),
				SecurityGroups: aws.StringSlice([]string{gid}),
				Subnets:        aws.StringSlice(createSubnets(clusterName)),
			},
		},
	}
	result, err := svc.CreateService(input)
	if err != nil {
		return store, err
	}

	store.Service = result
	return store, nil
}

// DeleteECSService ...
func DeleteECSService(creds AwsCreds, region, name, clusterName string, store ECSStore) (*ecs.DeleteServiceOutput, error) {
	sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(creds.ID, creds.secret, creds.token),
		})
	if err != nil {
		return nil, err
	}
	svc := ecs.New(sess)

	// TODO:(anoop) Stop services before deleting

	// De-Register task definition
	taskRev := strconv.FormatInt(*store.Task.TaskDefinition.Revision, 10)
	if _, err := deregisterTaskDefinition(svc, *store.Task.TaskDefinition.Family, taskRev); err != nil {
		return nil, err
	}

	// Delete the service
	input := &ecs.DeleteServiceInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(name),
		Force:   aws.Bool(true),
	}
	result, err := svc.DeleteService(input)
	if err != nil {
		return nil, err
	}

	// Delete security group
	if err := deleteSecurityGroup(*store.SecurityGroup.GroupId, region); err != nil {
		return nil, err
	}
	return result, nil
}

func registerTaskDefinition(svc *ecs.ECS, taskName string, mfsType MfsType) (*ecs.RegisterTaskDefinitionOutput, error) {
	image := "quay.io/tuxera/" + mfsType.name + ":" + mfsType.version
	input := &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(taskName), // Task Name
		Cpu:                     aws.String("256"),    // 0.25vCPU
		Memory:                  aws.String("512"),    // 512MB
		NetworkMode:             aws.String("awsvpc"),
		RequiresCompatibilities: aws.StringSlice([]string{"FARGATE"}),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Essential: aws.Bool(true),
				Image:     aws.String(image),
				Name:      aws.String("moosefs-server"),
			},
		},
	}
	result, err := svc.RegisterTaskDefinition(input)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func deregisterTaskDefinition(svc *ecs.ECS, taskName, revision string) (*ecs.DeregisterTaskDefinitionOutput, error) {
	input := &ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: aws.String(taskName + ":" + revision),
	}
	result, err := svc.DeregisterTaskDefinition(input)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func createSecurityGroup(name, desc, region string) (*ec2.CreateSecurityGroupOutput, error) {
	// create svc
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		return nil, err
	}
	// Create an EC2 service client.
	svc := ec2.New(sess)

	// get VPC ID
	// Get a list of VPCs so we can associate the group with the first VPC.
	result, err := svc.DescribeVpcs(nil)
	if err != nil {
		return nil, err
	}
	if len(result.Vpcs) == 0 {
		return nil, errors.New("No VPCs found to associate security group with")
	}
	vpcID := aws.StringValue(result.Vpcs[0].VpcId)
	// Create vpc
	createRes, err := svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(desc),
		VpcId:       aws.String(vpcID),
	})
	if err != nil {
		return nil, err
	}
	_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupName: aws.String(name),
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("tcp").
				SetFromPort(9419).
				SetToPort(9421).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				}),
			(&ec2.IpPermission{}).
				SetIpProtocol("tcp").
				SetFromPort(22).
				SetToPort(22).
				SetIpRanges([]*ec2.IpRange{
					(&ec2.IpRange{}).
						SetCidrIp("0.0.0.0/0"),
				}),
		},
	})
	if err != nil {
		return nil, err
	}
	return createRes, nil
}

func deleteSecurityGroup(groupID, region string) error {
	// create svc
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		return err
	}
	// Create an EC2 service client.
	svc := ec2.New(sess)

	_, err = svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(groupID),
	})
	if err != nil {
		return err
	}
	return nil
}

func createSubnets(name string) []string {
	// TODO(anoop): Dynamic
	subnets := []string{"subnet-a47092ed"}
	return subnets
}

/*
func main() {
	CreateECS(
        "eu-west-1",
		"my_cluster",
}
*/

/*
sess, err := session.NewSession(&aws.Config{Region: aws.String("us-west-2")})
*/

/*
sess, err := session.NewSession(&aws.Config{
    Region:      aws.String("us-west-2"),
    Credentials: credentials.NewSharedCredentials("", "test-account"),
})
*/

/*
svc := ecs.New(session.New())
input := &ecs.CreateClusterInput{
    ClusterName: aws.String("my_cluster"),
}

result, err := svc.CreateCluster(input)
if err != nil {
    if aerr, ok := err.(awserr.Error); ok {
        switch aerr.Code() {
        case ecs.ErrCodeServerException:
            fmt.Println(ecs.ErrCodeServerException, aerr.Error())
        case ecs.ErrCodeClientException:
            fmt.Println(ecs.ErrCodeClientException, aerr.Error())
        case ecs.ErrCodeInvalidParameterException:
            fmt.Println(ecs.ErrCodeInvalidParameterException, aerr.Error())
        default:
            fmt.Println(aerr.Error())
        }
    } else {
        // Print the error, cast err to awserr.Error to get the Code and
        // Message from an error.
        fmt.Println(err.Error())
    }
    return
}

fmt.Println(result)
*/

/*

svc := ec2.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))
    // Specify the details of the instance that you want to create.
    runResult, err := svc.RunInstances(&ec2.RunInstancesInput{
        // An Amazon Linux AMI ID for t2.micro instances in the us-west-2 region
        ImageId:      aws.String("ami-e7527ed7"),
        InstanceType: aws.String("t2.micro"),
        MinCount:     aws.Int64(1),
        MaxCount:     aws.Int64(1),
    })

    if err != nil {
        log.Println("Could not create instance", err)
        return
    }

    log.Println("Created instance", *runResult.Instances[0].InstanceId)

    // Add tags to the created instance
    _ , errtag := svc.CreateTags(&ec2.CreateTagsInput{
        Resources: []*string{runResult.Instances[0].InstanceId},
        Tags: []*ec2.Tag{
            {
                Key:   aws.String("Name"),
                Value: aws.String("MyFirstInstance"),
            },
        },
    })
    if errtag != nil {
        log.Println("Could not create tags for instance", runResult.Instances[0].InstanceId, errtag)
        return
    }

	log.Println("Successfully tagged instance")

*/
