package io4edgecore

import (
	"fmt"
)

type FirmwareVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HardwareInventory struct {
	Name   string `json:"part_number"`
	Rev    int    `json:"major_version"`
	Serial string `json:"serial_number"`
}

var globalParamDefs = []ParameterDefinition{
	{
		Key:            "wifi-ssid",
		Description:    "WiFi SSID",
		DefaultValue:   "",
		MaxLen:         32,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "wifi-pw",
		Description:    "WiFi Password",
		DefaultValue:   "",
		MaxLen:         64,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "device-id",
		Description:    "Device ID",
		DefaultValue:   "",
		MaxLen:         64,
		RebootRequired: true,
		Validator:      nil,
	},
	{
		Key:            "wifi-low-rssi",
		Description:    "WiFi Low RSSI Threshold",
		DefaultValue:   "-100",
		MaxLen:         10,
		RebootRequired: true,
		Validator:      nil,
	},
}

type Device struct {
	// fw/hw
	fw   *FirmwareVersion
	hw   *HardwareInventory
	repl []string

	globalParams *ParameterSet
	cs           *CoreServer
}

func NewDevice(httpsPort int, deviceID string, corePass string, fw *FirmwareVersion, hw *HardwareInventory, additionalRoutes []RouteRegistrar) (*Device, error) {
	d := &Device{
		fw:   fw,
		hw:   hw,
		repl: []string{},
	}
	nvs, err := NewParamNamespace("global")
	if err != nil {
		return nil, err
	}
	ps, err := NewParameterSet(nvs, globalParamDefs)
	if err != nil {
		return nil, err
	}
	d.globalParams = ps
	ps.ParamSetSingle("device-id", deviceID)
	cs, err := NewCoreServer(d, fmt.Sprintf(":%d", httpsPort), corePass, additionalRoutes)
	if err != nil {
		return nil, err
	}
	d.cs = cs
	return d, nil
}
