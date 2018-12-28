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
	"regexp"
	"testing"
	"time"
)

func TestGetPublicIP4K8s(t *testing.T) {

	ipChan := make(chan string)
	go func() {
		ipChan <- GetPublicIP4K8s()
	}()
	timer := time.Tick(5 * time.Second)
	select {
	case ip := <-ipChan:
		if ip == "" {
			t.Errorf("IP Seems to be empty")
		} else {
			re := regexp.MustCompile(`([0-9]?[0-9][0-9]?)(\.([0-9]?[0-9][0-9]?)){3}(\/32)`)
			_str := re.FindString(ip + "/32")
			if _str != ip+"/32" {
				t.Errorf("Does not match: " + ip)
			}
		}
	case <-timer:
		t.Errorf("Took longer than expected")
	}

}
