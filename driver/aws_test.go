package driver

import (
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
)

var (
	region            = "eu-west-1"
	clusterName       = "moosefs_cluster"
	masterServiceName = "moosefs-master_service"
	chunkServiceName  = "moosefs-chunk_service"
	moosefsSg         = "moosefs-test-sg"
	mfsTypeMaster     = MfsType{name: "moosefs-master", version: "0.0.1"}
	creds             = AwsCreds{
		ID:     "ASIAVQDDCCHDQZ55HCI5",
		secret: "wkL1jJnjO7A8lIFuMXn78glEd1+n5cc+xSZgNtOW",
		token:  "FQoGZXIvYXdzEKH//////////wEaDLzu0jopze0h1KKApyKsAa1MEKaTdH/nxChu3qTinxFjcOEKeVzimkd6mdF4/4BuuRv6KQL+LJDCoExjEyeDyK1ZvHOB/udBI8T9dkJOu0fo4bpCXi180VSFscC+sGs23qUbrXs0jry5w8Z7CWS62VkjBE03SBTXZydE0yWblqMkjcMQpGP8NLMRTgt0MhXifAQJ3y1w5knsGtZ6sx6b2969s334suvbZyyfeL8TzHLJ3BOnBdpGx6ceNMkoneqT3AU=",
	}
)

func TestCreateECSCluster(t *testing.T) {
	result, err := CreateECSCluster(creds, region, clusterName)
	if err != nil {
		t.Errorf("Error occured: ", err)
	}
	if "moosefs_cluster" != *result.Cluster.ClusterName {
		t.Errorf("Cluster status check: ", "moosefs_cluster", *result.Cluster.ClusterName)
	}
	if "ACTIVE" != *result.Cluster.Status {
		t.Errorf("Cluster status check: ", "ACTIVE", *result.Cluster.Status)
	}

}

func TestCreateDeleteSecurityGroup(t *testing.T) {
	groups, err := createSecurityGroup(moosefsSg, "For testing moosefs Fargate", "eu-west-1")
	if err != nil {
		t.Errorf("Error occured creating security group:", err)
	}
	if err = deleteSecurityGroup(*groups[0].GroupId, "eu-west-1"); err != nil {
		t.Errorf("GroupID creation/deletion failed:", err)
	}
}

// Only for moosefs-master
func TestCreateECSService(t *testing.T) {
	_, err := CreateECSCluster(creds, region, clusterName)
	if err != nil {
		t.Errorf("Error occured while creating cluster: ", err)
	}
	// moosefs-master service
	mfsTypeMaster := MfsType{name: "moosefs-master", version: "0.0.1"}
	storeMaster, err := CreateECSService(creds, region, masterServiceName, clusterName, mfsTypeMaster)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}
	resultService := storeMaster.Service
	if resultService == nil || resultService.Service == nil {
		// already existing cluster
		if len(storeMaster.TaskList.TaskArns) < 0 {
			t.Errorf("Task Arns empty", storeMaster.TaskList)
		}
	} else {
		// newly created
		if masterServiceName != *storeMaster.Service.Service.ServiceName {
			t.Errorf("Wrong service name: ", "moosefs-server_service", *resultService.Service.ServiceName)
		}
	}
	result, err := DeleteECSService(creds, region, chunkServiceName, clusterName, storeMaster)
	if err != nil {
		t.Errorf("Error deleting service", err)
	}
	if "DRAINING" != *result.Service.Status {
		t.Errorf("Wrong service status: ", "DRAINING", *result.Service.Status)
	}
	DeleteECSCluster(creds, region, clusterName)
	/*
		// moosefs-chunk service
		mfsTypeChunk := MfsType{name: "moosefs-chunk", version: "0.0.1"}
		storeChunk, err := CreateECSService(creds, region, chunkServiceName, clusterName, mfsTypeChunk)
		if err != nil {
			t.Errorf("Error creating service:", err)
		}
		if chunkServiceName != *storeChunk.Service.Service.ServiceName {
			t.Errorf("Wrong service name: ", "moosefs-server_service", *storeChunk.Service.Service.ServiceName)
		}
	*/
	/*
	 */
}
func TestGetPublicIP4(t *testing.T) {
	// prepare

	_, err := CreateECSCluster(creds, region, clusterName)
	if err != nil {
		t.Errorf("Error occured while creating cluster: ", err)
	}

	// moosefs-master service

	storeMaster, err := CreateECSService(creds, region, masterServiceName, clusterName, mfsTypeMaster)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}

	// GetIp
	ip, err := GetPublicIP4(creds, region, clusterName, *storeMaster.TaskList.TaskArns[0])
	if err != nil || ip == nil {
		t.Errorf("Error GetIPv4()", err)
	}

	// cleanup
	_, err = DeleteECSService(creds, region, chunkServiceName, clusterName, storeMaster)
	if err != nil {
		t.Errorf("Error deleting service", err)
	}
}

func TestCreateDeleteEc2Instance(t *testing.T) {
	storeMaster, err := CreateECSService(creds, region, masterServiceName, clusterName, mfsTypeMaster)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}

	// GetIp
	ip, err := GetPublicIP4(creds, region, clusterName, *storeMaster.TaskList.TaskArns[0])
	if err != nil || ip == nil {
		t.Errorf("Error GetIPv4()", err)
	}
	res, err := CreateEc2Instance(region, clusterName, *ip, 100)
	if err != nil {
		t.Errorf("Error occured creating instance:", err)
	}
	t.Errorf("res", res)
	/*
		if err = deleteSecurityGroup(*groups[0].GroupId, "eu-west-1"); err != nil {
			t.Errorf("GroupID creation/deletion failed:", err)
		}
	*/

}

func TestDeleteECSCluster(t *testing.T) {
	result, err := DeleteECSCluster(creds, region, clusterName)
	if err != nil {
		t.Errorf("Error occured: ", err)
	}
	if "moosefs_cluster" != *result.Cluster.ClusterName {
		t.Errorf("Cluster status check: ", "moosefs_cluster", *result.Cluster.ClusterName)
	}
	if "INACTIVE" != *result.Cluster.Status {
		t.Errorf("Cluster status check: ", "INACTIVE", *result.Cluster.Status)
	}
}

func TestRegisterDeregisterTaskDefinition(t *testing.T) {
	sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(creds.ID, creds.secret, creds.token),
		})
	if err != nil {
		t.Errorf("Creating session failed: ", err)
	}

	svc := ecs.New(sess)
	mfsType := MfsType{
		name:    "moosefs-master",
		version: "0.0.1",
		Env: []*ecs.KeyValuePair{
			&ecs.KeyValuePair{
				Name:  aws.String("mfsmaster"),
				Value: aws.String("8.8.8.8"),
			},
		},
	}
	result, err := registerTaskDefinition(svc, mfsType)
	if err != nil {
		t.Errorf("Register task definition failed: ", err)
	}
	if "ACTIVE" != *result.TaskDefinition.Status {
		t.Errorf("Task definition registration status: ", "ACTIVE", *result.TaskDefinition.Status)
	}
	t.Errorf("result", result)
	result1, err := deregisterTaskDefinition(svc, mfsType.name, strconv.FormatInt(*result.TaskDefinition.Revision, 10))
	if err != nil {
		t.Errorf("Register task definition failed: ", err)
	}
	if "INACTIVE" != *result1.TaskDefinition.Status {
		t.Errorf("Task definition registration status: ", "INACTIVE", *result1.TaskDefinition.Status)
	}
}

/*
# Install fuse and others
apt-get update && apt-get install -y wget gnupg2 fuse libfuse2 ca-certificates e2fsprogs
# Install certificates
wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
. /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list
# Install chunkserver
apt-get update && apt-get install -y moosefs-chunkserver
# For testing
mkdir -p /mnt/sdb1 && chown -R mfs:mfs /mnt/sdb1 && echo "/mnt/sdb1 1GiB" >> /etc/mfs/mfshdd.cfg
# Start the chunkserver service
systemctl start moosefs-chunkserver


--------------

apt-get update && apt-get install -y wget gnupg2 fuse libfuse2 ca-certificates e2fsprogs

wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
. /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

apt-get update && apt-get install -y moosefs-client




AMI Name: amzn-ami-hvm-2018.03.0.20180412-x86_64-ebs

*/
