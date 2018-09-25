/*
   Copyright 2018 Tuxera Oy. All Rights Reserved.

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

package driver

import (
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
)

var (
	region = "eu-west-1"
	//	clusterName       = "moosefs_cluster"
	//	masterServiceName = "moosefs-master_service"
	masterServiceName = "pvc-test-82daf9b5-ac54-11e8-a108-42010aa401ac"
	clusterName       = "pvc-test-82daf9b5-ac54-11e8-a108-42010aa401ac"
	chunkServiceName  = "moosefs-chunk_service"
	moosefsSg         = "moosefs-test-sg"
	creds             = AwsCreds{
	}
	d = &Driver{
		log: logrus.New().WithFields(logrus.Fields{
			"purpose": "testing",
		}),
		awsAccessKey:    creds.ID,
		awsSecret:       creds.secret,
		awsSessionToken: creds.token,
		awsRegion:       region,
	}
)

func TestCreateECSCluster(t *testing.T) {
	// Create AWS Session
	sess, err := CreateAWSSession(d)

	result, err := CreateECSCluster(sess, clusterName)
	if err != nil {
		t.Errorf("Error occured: ", err)
	}
	if clusterName != *result.Cluster.ClusterName {
		t.Errorf("Cluster status check: ", clusterName, *result.Cluster.ClusterName)
	}
	if "ACTIVE" != *result.Cluster.Status {
		t.Errorf("Cluster status check: ", "ACTIVE", *result.Cluster.Status)
	}

}

func TestCreateDeleteSecurityGroup(t *testing.T) {
	// Create AWS Session
	sess, err := CreateAWSSession(d)

	groups, err := createSecurityGroup(moosefsSg, "For testing moosefs Fargate", d.awsRegion, sess)
	if err != nil {
		t.Errorf("Error occured creating security group:", err)
	}
	if err = deleteSecurityGroup(*groups[0].GroupId, d.awsRegion); err != nil {
		t.Errorf("GroupID creation/deletion failed:", err)
	}
}

// Only for moosefs-master
func TestCreateECSService(t *testing.T) {
	// Create AWS Session
	sess, err := CreateAWSSession(d)

	_, err = CreateECSCluster(sess, clusterName)
	if err != nil {
		t.Errorf("Error occured while creating cluster: ", err)
	}
	// moosefs-master service
	storeMaster, err := CreateECSService(sess, d, masterServiceName, clusterName, mfsTypeMaster)
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
	/*
		result, err := DeleteECSService(creds, region, masterServiceName, clusterName, storeMaster)
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
	// Create AWS Session
	sess, err := CreateAWSSession(d)

	_, err = CreateECSCluster(sess, clusterName)
	if err != nil {
		t.Errorf("Error occured while creating cluster: ", err)
	}

	// moosefs-master service

	storeMaster, err := CreateECSService(sess, d, masterServiceName, clusterName, mfsTypeMaster)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}

	// GetIp
	ip, err := GetPublicIP4(sess, region, clusterName, *storeMaster.TaskList.TaskArns[0])
	if err != nil || ip == nil {
		t.Errorf("Error GetIPv4()", err)
	}

	// cleanup
	_, err = DeleteECSService(sess, region, chunkServiceName, clusterName, storeMaster)
	if err != nil {
		t.Errorf("Error deleting service", err)
	}
}

func TestCreateDeleteEc2Instance(t *testing.T) {

	// Create AWS Session
	sess, err := CreateAWSSession(d)

	storeMaster, err := CreateECSService(sess, d, masterServiceName, clusterName, mfsTypeMaster)
	if err != nil {
		t.Errorf("Error creating service:", err)
	}

	// GetIp
	ip, err := GetPublicIP4(sess, region, clusterName, *storeMaster.TaskList.TaskArns[0])
	if err != nil || ip == nil {
		t.Errorf("Error GetIPv4()", err)
	}

	_, err = CreateEc2Instance(d, clusterName, *ip, *ip+":9412", 100, sess)
	if err != nil {
		t.Errorf("Error occured creating instance:", err)
	}

	// Idempotency
	_, err = CreateEc2Instance(d, clusterName, *ip, *ip+":9412", 100, sess)
	if err != nil {
		t.Errorf("Error occured creating instance:", err)
	}

	// Delete
	_, err = DeleteEc2Instance(*ip+":9412", d, sess)
	if err != nil {
		t.Errorf("Deleting Ec2 instance failed: ", err)
	}

	_, err = DeleteECSService(sess, region, masterServiceName, clusterName, storeMaster)
	if err != nil {
		t.Errorf("Deleting ECS service failed: ", err)
	}
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
	// Create AWS Session
	sess, err := CreateAWSSession(d)
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

func TestEncodedUserData(t *testing.T) {
	userDataEncoded := encodedUserData(100, "/dev/xvhd", "0.0.0.0")
	actuals := "CmN1cmwgaHR0cHM6Ly9naXN0LmdpdGh1YnVzZXJjb250ZW50LmNvbS9tYW5pYW5rYXJhL2Q0Y2Q2ZWEzNjQ5NmFmNmU1N2IzMzMzYzFlODgyODI4L3Jhdy9mZGYwN2MwOWYyNWNkM2JmMTZjNTY3MTZkOTVhZTVlZWM0ODUzZWIzL3Byb3Zpc2lvbi1tb29zZWZzLnNoPmluaXQuc2gKY2htb2QgYSt4IGluaXQuc2gKLi9pbml0LnNoIDAuMC4wLjAgMTAwIC9kZXYveHZoZAoJCQ=="
	if userDataEncoded != actuals {
		t.Errorf("Wrong userData encoding for: ", userDataEncoded, actuals)
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
