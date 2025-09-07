package io4edgecore

import (
	"errors"
	"os"

	yaml "gopkg.in/yaml.v2"
)

var ErrParameterNotFound = errors.New("parameter not found")
// Simulate Non volatile Storage (NVS) for parameters
// Use a file with key=value pairs rather than a real NVS (flash)
// Use YAML in the file

type ParamNamespace struct {
	name     string
	params   map[string]string
	fileName string
}

func NewParamNamespace(name string) (*ParamNamespace, error) {
	if name == "" {
		return nil, errors.New("namespace name is empty")
	}
	fileName := name + "_params.yaml"
	ns := &ParamNamespace{
		name:     name,
		params:   make(map[string]string),
		fileName: fileName,
	}
	err := ns.load()
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (ns *ParamNamespace) Name() string {
	return ns.name
}

func (ns *ParamNamespace) load() error {
	// Load parameters from the YAML file
	data, err := os.ReadFile(ns.fileName)
	if err != nil {
		// if file does not exist, create file
		if os.IsNotExist(err) {
			ns.params = make(map[string]string)
			_, err := os.Create(ns.fileName)
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, &ns.params)
}

func (ns *ParamNamespace) save() error {
	// Save parameters to the YAML file
	data, err := yaml.Marshal(ns.params)
	if err != nil {
		return err
	}
	return os.WriteFile(ns.fileName, data, 0644)
}

func (ns *ParamNamespace) GetParam(key string) (string, error) {
	value, exists := ns.params[key]
	if !exists {
		return "", ErrParameterNotFound
	}
	return value, nil
}

func (ns *ParamNamespace) HasKey(key string) bool {
	_, exists := ns.params[key]
	return exists
}

func (ns *ParamNamespace) SetParam(key string, value string) error {
	ns.params[key] = value
	return ns.save()
}

func (ns *ParamNamespace) DeleteParam(key string) error {
	_, exists := ns.params[key]
	if !exists {
		return ErrParameterNotFound
	}
	delete(ns.params, key)
	return ns.save()
}

func (ns *ParamNamespace) Erase() error {
	ns.params = make(map[string]string)
	return ns.save()
}

func (ns *ParamNamespace) ListParams() map[string]string {
	// Return a copy of the parameters map to avoid external modification
	copy := make(map[string]string)
	for k, v := range ns.params {
		copy[k] = v
	}
	return copy
}
