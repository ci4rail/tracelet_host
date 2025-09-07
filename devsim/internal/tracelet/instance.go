/*
Copyright © 2022 Ci4Rail GmbH
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

package tracelet

import (
	"sync"

	"github.com/ci4rail/tracelet_host/devsim/pkg/io4edge_core"
)

// Tracelet represents the tracelet functionality
type Tracelet struct {
	loc      location
	locMutex sync.Mutex // mutex to protect loc
	deviceID string
	coreDev     *io4edgecore.Device
}

// NewInstance creates a new Easylocate simulator instance
func NewInstance(deviceID string, locationServerAddress string) (*Tracelet, error) {
	e := &Tracelet{
		deviceID: deviceID,
	}
	coreDev, err := io4edgecore.NewDevice(
		&io4edgecore.FirmwareVersion{
			Name:    "tracelet",
			Version: "1.0.0",
		},
		&io4edgecore.HardwareInventory{
			Name:   "devsim",
			Rev:    1,
			Serial: deviceID,
		},
	)
	if err != nil {
		return nil, err
	}
	e.coreDev = coreDev
	// start core server	
	err = e.locationClient(locationServerAddress)
	if err != nil {
		return nil, err
	}
	e.locationGenerator()

	return e, nil
}
