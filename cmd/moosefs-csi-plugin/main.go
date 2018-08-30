package main

import (
	"flag"
	"log"

	"github.com/moosefs/moosefs-csi/driver"
)

func main() {
	var (
		endpoint        = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/com.tuxera.csi.moosefs/csi.sock", "CSI endpoint")
		awsAccessKeyID  = flag.String("aws-access", "", "AWS Access key Id")
		awsSessionToken = flag.String("aws-session", "", "AWS Session token")
		awsSecret       = flag.String("aws-secret", "", "AWS Secret Access key")
		awsRegion       = flag.String("aws-region", "eu-west-1", "AWS region where to deploy")
	)
	flag.Parse()

	drv, err := driver.NewDriver(*endpoint, awsAccessKeyID, awsSecret, awsSessionToken, awsRegion)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}
