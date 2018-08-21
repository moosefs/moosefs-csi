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
	region      = "eu-west-1"
	clusterName = "moosefs_cluster"
	serviceName = "moosefs-server_service"
	taskName    = "moosefs-server_task" // must be unique
	creds       = AwsCreds{
		ID:     "",
		secret: "+yak1k/saFJarvoLb6",
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
	mfsType := MfsType{name: "moosefs-master", version: "0.0.1"}
	result, err := registerTaskDefinition(svc, taskName, mfsType)
	if err != nil {
		t.Errorf("Register task definition failed: ", err)
	}
	if "ACTIVE" != *result.TaskDefinition.Status {
		t.Errorf("Task definition registration status: ", "ACTIVE", *result.TaskDefinition.Status)
	}
	result1, err := deregisterTaskDefinition(svc, taskName, strconv.FormatInt(*result.TaskDefinition.Revision, 10))
	if err != nil {
		t.Errorf("Register task definition failed: ", err)
	}
	if "INACTIVE" != *result1.TaskDefinition.Status {
		t.Errorf("Task definition registration status: ", "INACTIVE", *result1.TaskDefinition.Status)
	}
}

func TestCreateDeleteECSService(t *testing.T) {
	_, err := CreateECSCluster(creds, region, clusterName)
	if err != nil {
		t.Errorf("Error occured while creating cluster: ", err)
	}
	mfsType := MfsType{name: "moosefs-master", version: "0.0.1"}
	store, err := CreateECSService(creds, region, serviceName, clusterName, taskName, mfsType)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}
	resultService := store.Service
	if "moosefs-server_service" != *resultService.Service.ServiceName {
		t.Errorf("Wrong service name: ", "moosefs-server_service", *resultService.Service.ServiceName)
	}

	result, err := DeleteECSService(creds, region, serviceName, clusterName, store)
	if err != nil {
		t.Errorf("Error deleting service", err)
	}
	if "DRAINING" != *result.Service.Status {
		t.Errorf("Wrong service name: ", "moosefs-server_service", *result.Service.Status)
	}
	DeleteECSCluster(creds, region, clusterName)

}

func TestDeleteECSService(t *testing.T) {
}

func TestCreateDeleteSecurityGroup(t *testing.T) {
	resp, err := createSecurityGroup("moosefs-test1", "For testing moosefs Fargate", "eu-west-1")
	if err != nil {
		t.Errorf("Error occured:", err)
	}
	if err = deleteSecurityGroup(*resp.GroupId, "eu-west-1"); err != nil {
		t.Errorf("GroupID creation/deletion failed:", err)
	}
}
