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
	"fmt"
	"sync"

	io4edgecore "github.com/ci4rail/tracelet_host/devsim/pkg/io4edge_core"
	lsiclient "github.com/ci4rail/tracelet_host/devsim/pkg/lsi_client"
)

// Tracelet represents the tracelet functionality
type Tracelet struct {
	loc            location
	locMutex       sync.Mutex // mutex to protect loc
	deviceID       string
	IPv4Address    string
	coreDev        *io4edgecore.Device
	posParams      *io4edgecore.ParameterSet
	mode           Mode
	trackFile      string
	replayMessages ReplayMessages
	lsiClient      *lsiclient.Instance
}

type Mode string

const (
	ModeRandom Mode = "random"
	ModeReplay Mode = "replay"
)

type Config struct {
	DeviceID              string
	LocationServerAddress string
	IPv4Address           string
	HTTPSPort             int
	CorePass              string
	Mode                  Mode
	TrackFile             string
}

var posParamDefs = []io4edgecore.ParameterDefinition{
	{
		Key:            "ntrip-caster",
		Description:    "NTRIP Caster address:port:mountpoint",
		DefaultValue:   "",
		MaxLen:         100,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:             "ntrip-creds",
		Description:     "NTRIP Credentials user:password",
		DefaultValue:    "",
		IsReadProtected: true,
		MaxLen:          100,
		RebootRequired:  true,
		Validator:       nil,
	},
	{
		Key:            "ntrip-hostname",
		Description:    "NTRIP Caster hostname for Rev 2 authentication",
		DefaultValue:   "",
		MaxLen:         100,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "gnss-rate",
		Description:    "GNSS PVAT update rate in Hz",
		DefaultValue:   "1",
		MaxLen:         1,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "gnss-dr",
		Description:    "GNSS dead reckoning and sensor fusion on/off",
		DefaultValue:   "on",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "gnss-dynmodel",
		Description:    "GNSS dynamic platform model",
		DefaultValue:   "",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "gnss-automntalg",
		Description:    "GNSS auto mount alignment on/off",
		DefaultValue:   "off",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "tacho-k",
		Description:    "Tacho impulse constant in pulses per kilometer",
		DefaultValue:   "4000",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "dev2vrp",
		Description:    "Device to vehicle rotation point settings x:y:z in cm",
		DefaultValue:   "0:0:0",
		MaxLen:         32,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "dev-height",
		Description:    "Device mounting height from floor in cm",
		DefaultValue:   "280",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "dev-alg",
		Description:    "Device alignment settings yaw:pitch:roll in degrees",
		DefaultValue:   "0:0:0",
		MaxLen:         32,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "use-wt",
		Description:    "Use wheel tick on/off",
		DefaultValue:   "off",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-motcheck",
		Description:    "UWB motion check rate",
		DefaultValue:   "1",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-server",
		Description:    "UWB UART server on/off",
		DefaultValue:   "off",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-fw-update",
		Description:    "UWB firmware update on/off",
		DefaultValue:   "on",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-gnss-pos",
		Description:    "Transmit GNSS position to UWB module on/off",
		DefaultValue:   "on",
		MaxLen:         3,
		RebootRequired: false,
		Validator:      nil,
	},
	{
		Key:            "uwb-auto-reset",
		Description:    "UWB module auto reset on/off",
		DefaultValue:   "on",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-corr-blob",
		Description:    "Base64 encoded UWB correlation blob",
		DefaultValue:   "",
		MaxLen:         32767,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "uwb-test-mode",
		Description:    "UWB test mode on/off",
		DefaultValue:   "off",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "site-origins",
		Description:    "Site origins list siteid:lat:lon:azimuth separated by semicolons",
		DefaultValue:   "",
		MaxLen:         4096,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "loc-srv",
		Description:    "Location Server address:port",
		DefaultValue:   "",
		MaxLen:         64,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "sleep-mode",
		Description:    "Sleep manager on/off",
		DefaultValue:   "on",
		MaxLen:         3,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "sleep-wakeintv",
		Description:    "Wake-up interval in seconds",
		DefaultValue:   "600",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "fuse-rate",
		Description:    "Fused position update rate in Hz",
		DefaultValue:   "1",
		MaxLen:         1,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "fuse-uwb-th",
		Description:    "Maximum UWB eph threshold for fused position in meters",
		DefaultValue:   "5",
		MaxLen:         16,
		RebootRequired: true,
		Validator:      nil,
	},
}

// NewInstance creates a new tracelet simulator instance
func NewInstance(deviceID string, locationServerAddress string, IPv4Address string, httpsPort int) (*Tracelet, error) {
	return NewInstanceWithConfig(Config{
		DeviceID:              deviceID,
		LocationServerAddress: locationServerAddress,
		IPv4Address:           IPv4Address,
		HTTPSPort:             httpsPort,
		Mode:                  ModeRandom,
	})
}

func NewInstanceWithConfig(cfg Config) (*Tracelet, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeRandom
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	tl := &Tracelet{
		locMutex:    sync.Mutex{},
		deviceID:    cfg.DeviceID,
		IPv4Address: cfg.IPv4Address,
		mode:        cfg.Mode,
		trackFile:   cfg.TrackFile,
	}

	if cfg.Mode == ModeReplay {
		replayMessages, err := loadReplayMessages(cfg.TrackFile, cfg.DeviceID, cfg.IPv4Address)
		if err != nil {
			return nil, err
		}
		tl.replayMessages = replayMessages
	}

	nvs, err := io4edgecore.NewParamNamespace("pos")
	if err != nil {
		return nil, err
	}
	ps, err := io4edgecore.NewParameterSet(nvs, posParamDefs)
	if err != nil {
		return nil, err
	}
	tl.posParams = ps

	coreDev, err := io4edgecore.NewDevice(
		cfg.HTTPSPort,
		cfg.DeviceID,
		cfg.CorePass,
		&io4edgecore.FirmwareVersion{
			Name:    "tracelet",
			Version: "1.0.0",
		},
		&io4edgecore.HardwareInventory{
			Name:   "devsim",
			Rev:    1,
			Serial: cfg.DeviceID,
		},
		[]io4edgecore.RouteRegistrar{
			RegistrarTracelet(tl),
		},
	)
	if err != nil {
		return nil, err
	}
	tl.coreDev = coreDev
	err = tl.locationClient(cfg.LocationServerAddress)
	if err != nil {
		return nil, err
	}
	if tl.mode == ModeRandom {
		tl.locationGenerator()
	}

	return tl, nil
}

func (cfg Config) validate() error {
	switch cfg.Mode {
	case ModeRandom:
		return nil
	case ModeReplay:
		if cfg.TrackFile == "" {
			return fmt.Errorf("track file is required in replay mode")
		}
		return nil
	default:
		return fmt.Errorf("unsupported tracelet mode %q", cfg.Mode)
	}
}
