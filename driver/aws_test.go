package driver

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func createSession() *session.Session {
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String("eu-west-2"),
	}))
	return sess
}

func TestSubnet(t *testing.T) {

	svc := ec2.New(createSession())

	result, err := getDefaultSubnet(svc)
	if err != nil {
		t.Error("Obtained error: ", err)
	}
	if result != nil {
		t.Error("Obtained subnet: ", *result)
	}
}
