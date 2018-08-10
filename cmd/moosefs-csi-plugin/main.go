package main

import (
	"flag"
	"log"

	"github.com/moosefs/moosefs-csi/driver"
)

func main() {
	var (
		endpoint  = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/com.tuxera.csi.moosefs/csi.sock", "CSI endpoint")
		masterURL = flag.String("url", "mfsmaster:", "MooseFS master url")
	)
	flag.Parse()

	drv, err := driver.NewDriver(*endpoint, *masterURL)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}
