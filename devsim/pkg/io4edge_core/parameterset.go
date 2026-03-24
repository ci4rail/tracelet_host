package io4edgecore

// A parameterset is a collection of parameters store in a NVS namespace

import (
	"errors"
	"strings"
)

var ErrInvalidParameter = errors.New("invalid parameter")
var ErrReadProtected = errors.New("read protected parameter")

const (
	versionParamKey = "__version"
	versionFactory  = "factory"
	readProtected   = "(read_protected)"
	versionModified = " (modified)"
)

type ParameterRoot struct {
	parameterSets  map[string]*ParameterSet
	rebootRequired bool
}

var parameterRoot ParameterRoot

type ParameterValidator func(def *ParameterDefinition, value string) bool

type ParameterDefinition struct {
	Key             string
	DefaultValue    string
	Description     string
	IsReadProtected bool
	MaxLen          int
	RebootRequired  bool
	Validator       ParameterValidator
}

type ParameterInstance struct {
	definition ParameterDefinition
	ps         *ParameterSet
}

type ParameterSetGetRV struct {
	Version        string
	Params         map[string]string
	Missing        []string
	Unsupported    []string
	Invalid        []string
	RebootRequired bool
}

type ParameterSetSetRV struct {
	Missing        []string
	Unsupported    []string
	RebootRequired bool
}

type ParameterSetListEntry struct {
	Name 	  string 
	Description string
	Default 	  string
	ReadProtected bool
	Persistence   string
}

type ParameterSetListRV []ParameterSetListEntry

type ParameterSet struct {
	nvsNS *ParamNamespace
	param map[string]*ParameterInstance
}


func NewParameterSet(nvsNS *ParamNamespace, definitions []ParameterDefinition) (*ParameterSet, error) {
	if nvsNS == nil {
		return nil, ErrInvalidParameter
	}
	if len(definitions) == 0 {
		return nil, ErrInvalidParameter
	}
	ps := &ParameterSet{
		nvsNS: nvsNS,
		param: make(map[string]*ParameterInstance),
	}
	for _, def := range definitions {
		if def.Key == "" {
			return nil, ErrInvalidParameter
		}
		ps.param[def.Key] = &ParameterInstance{
			definition: def,
			ps:         ps,
		}
	}
	_, err := nvsNS.GetParam(versionParamKey)
	if err != nil {
		nvsNS.Erase()
	}
	_, exists := parameterRoot.parameterSets[nvsNS.Name()]
	if !exists {
		parameterRoot.parameterSets[nvsNS.Name()] = ps
	} else {
		return nil, errors.New("parameter set already exists")
	}
	return ps, nil
}

func (ps *ParameterSet) ListParams() (ParameterSetListRV, error) {
	var rv ParameterSetListRV
	for _, p := range ps.param {
		entry := ParameterSetListEntry{
			Name:          p.definition.Key,
			Description:   p.definition.Description,
			Default:       p.definition.DefaultValue,
			ReadProtected: p.definition.IsReadProtected,
			Persistence:   "ESP_NVS",
		}
		rv = append(rv, entry)
	}
	return rv, nil
}

func (ps *ParameterSet) getMissing() []string {
	var missing []string
	for key := range ps.param {
		if !ps.nvsNS.HasKey(key) {
			missing = append(missing, key)
		}
	}
	return missing
}

func ValidateParameterValue(def *ParameterDefinition, value string) bool {
	if def.MaxLen > 0 && len(value) > def.MaxLen {
		return false
	}
	if def.Validator != nil {
		return def.Validator(def, value)
	}
	return true
}

func (p *ParameterInstance) ParamIsValid() bool {
	value, err := p.ps.nvsNS.GetParam(p.definition.Key)
	if err != nil {
		return err == ErrParameterNotFound
	}
	return ValidateParameterValue(&p.definition, value)
}

func (p *ParameterInstance) ParamGet() (string, error) {
	if !p.ParamIsValid() {
		return "", ErrInvalidParameter
	}
	if p.definition.IsReadProtected {
		return readProtected, nil
	}

	value, err := p.ps.nvsNS.GetParam(p.definition.Key)
	if err != nil {
		if err == ErrParameterNotFound {
			return p.definition.DefaultValue, nil
		}
		return "", err
	}
	return value, nil
}

func (ps *ParameterSet) ParamSetSingle(key string, value string) (bool, error) {
	p, exists := ps.param[key]
	if !exists {
		return parameterRoot.rebootRequired, ErrParameterNotFound
	}
	if err := p.paramSetLL(value); err != nil {
		return parameterRoot.rebootRequired, err
	}
	version, err := p.ps.nvsNS.GetParam(versionParamKey)
	if err != nil {
		version = versionFactory
	}
	if strings.HasSuffix(version, versionModified) {
		return parameterRoot.rebootRequired, nil
	}
	version = version + versionModified
	err = p.ps.nvsNS.SetParam(versionParamKey, version)
	if err != nil {
		return parameterRoot.rebootRequired, err
	}
	return parameterRoot.rebootRequired, nil
}

func (ps *ParameterSet) ParamGetSingle(key string) (string, error) {
	p, exists := ps.param[key]
	if !exists {
		return "", ErrParameterNotFound
	}
	return p.ParamGet()
}

func (p *ParameterInstance) paramSetLL(value string) error {
	if !ValidateParameterValue(&p.definition, value) {
		return ErrInvalidParameter
	}
	if err := p.ps.nvsNS.SetParam(p.definition.Key, value); err != nil {
		return err
	}
	if p.definition.RebootRequired {
		parameterRoot.rebootRequired = true
	}
	return nil
}

func (ps *ParameterSet) ParameterSetGet() (*ParameterSetGetRV, error) {
	rv := &ParameterSetGetRV{
		Params:      make(map[string]string),
		Unsupported: []string{},
		Invalid:     []string{},
		Missing:     []string{},
	}
	version, err := ps.nvsNS.GetParam(versionParamKey)
	if err != nil {
		rv.Version = versionFactory
	} else {
		rv.Version = version
	}

	m := ps.nvsNS.ListParams()
	for key := range m {
		if strings.HasPrefix(key, "__") {
			continue
		}
		_, exists := ps.param[key]
		if !exists {
			rv.Unsupported = append(rv.Unsupported, key)
			continue
		}
	}
	rv.Missing = ps.getMissing()

	for key, p := range ps.param {
		if !p.ParamIsValid() {
			rv.Invalid = append(rv.Invalid, key)
			continue
		}
		value, err := p.ParamGet()
		if err != nil {
			if err == ErrReadProtected {
				value = readProtected
			} else {
				return nil, err
			}
		}
		rv.Params[key] = value
	}
	rv.RebootRequired = parameterRoot.rebootRequired
	return rv, nil
}

func (ps *ParameterSet) ParameterSetSet(version string, params map[string]string) (*ParameterSetSetRV, error) {
	rv := &ParameterSetSetRV{
		Unsupported:    []string{},
		Missing:        []string{},
		RebootRequired: true,
	}
	for key, value := range params {
		_, exists := ps.param[key]
		if !exists {
			continue
		}
		if !ValidateParameterValue(&ps.param[key].definition, value) {
			return nil, ErrInvalidParameter
		}
	}
	if err := ps.nvsNS.Erase(); err != nil {
		return nil, err
	}
	for key, value := range params {
		_, exists := ps.param[key]
		if !exists {
			err := ps.nvsNS.SetParam(key, value)
			if err != nil {
				return nil, err
			}
			rv.Unsupported = append(rv.Unsupported, key)
			continue
		}
		if err := ps.param[key].paramSetLL(value); err != nil {
			return nil, err
		}
	}
	rv.Missing = ps.getMissing()

	err := ps.nvsNS.SetParam(versionParamKey, version)
	if err != nil {
		return nil, err
	}
	rv.RebootRequired = parameterRoot.rebootRequired
	return rv, nil
}


func ParameterSetsPurge() {
	parameterRoot = ParameterRoot{
		parameterSets:  make(map[string]*ParameterSet),
		rebootRequired: false,
	}
}

func ParameterSetForceFactoryDefaults() {
	for _, ps := range parameterRoot.parameterSets {
		ps.nvsNS.Erase()
	}
}

func init() {
	ParameterSetsPurge()
}
