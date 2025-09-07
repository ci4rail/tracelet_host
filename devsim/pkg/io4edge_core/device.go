package io4edgecore

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
	fw   FirmwareVersion
	hw   HardwareInventory
	repl []string

	globalParams *ParameterSet
	cs 		  *CoreServer
}

func NewDevice(fw *FirmwareVersion, hw *HardwareInventory) (*Device, error) {
	d := &Device{
		fw: *fw,
		hw: *hw,
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

	cs, err := NewCoreServer(d, ":9443")
	if err != nil {
		return nil, err
	}
	d.cs = cs
	return d, nil
}
