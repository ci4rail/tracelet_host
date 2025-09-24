package io4edgecore

import (
	"encoding/json"
	"fmt"
	"os"
)

type FirmwareVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HardwareInventory struct {
	Name   string `json:"name"`
	Rev    int    `json:"rev"`
	Serial string `json:"serial"`
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
}

type Device struct {
	// fw/hw
	fw   *FirmwareVersion
	hw   *HardwareInventory
	repl []string

	globalParams *ParameterSet
	cs           *CoreServer
}

// read firmware name and version from firmware file
func Firmware() FirmwareVersion {
	f, err := os.Open(firmwareFile)
	if err != nil {
		return FirmwareVersion{}
	}
	defer f.Close()

	var fw FirmwareVersion
	if err := json.NewDecoder(f).Decode(&fw); err != nil {
		return FirmwareVersion{}
	}
	return fw
}

func NewDevice(httpsPort int, fw *FirmwareVersion, hw *HardwareInventory, additionalRoutes []RouteRegistrar) (*Device, error) {
	fwFromFile := Firmware()
	if fwFromFile.Name != "" && fwFromFile.Version != "" {
		fw = &fwFromFile
	}

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

	cs, err := NewCoreServer(d, fmt.Sprintf(":%d", httpsPort), additionalRoutes)
	if err != nil {
		return nil, err
	}
	d.cs = cs
	return d, nil
}
