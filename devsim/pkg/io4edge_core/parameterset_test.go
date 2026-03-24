package io4edgecore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParameterSet(t *testing.T) {
	nvs, err := NewParamNamespace("test")
	assert.NotNil(t, nvs)
	assert.NoError(t, err)
	nvs.Erase()

	defs := []ParameterDefinition{
		{
			Key:            "param1",
			Description:    "a test parameter",
			DefaultValue:   "default1",
			MaxLen:         10,
			RebootRequired: true,
			Validator:      nil,
		},
		{
			Key:            "param2",
			Description:    "a test parameter with validator",
			DefaultValue:   "val2",
			MaxLen:         5,
			RebootRequired: true,
			Validator: func(def *ParameterDefinition, value string) bool {
				return strings.HasPrefix(value, "val")
			},
		},
	}

	ps, err := NewParameterSet(nvs, defs)
	assert.NotNil(t, ps)
	assert.NoError(t, err)

	rv, err := ps.ParameterSetGet()
	assert.NotNil(t, rv)
	assert.NoError(t, err)
	assert.Equal(t, "factory", rv.Version)
	assert.Equal(t, 2, len(rv.Params))
	assert.Equal(t, "default1", rv.Params["param1"])
	assert.Equal(t, "val2", rv.Params["param2"])
	assert.Equal(t, 2, len(rv.Missing))
	assert.Equal(t, 0, len(rv.Unsupported))
	assert.Equal(t, 0, len(rv.Invalid))

	rv2, err := ps.ParameterSetSet("1.0.0", map[string]string{
		"param1": "newvalue",
		"param2": "val23",
		"param3": "othervalue",
	})
	assert.NotNil(t, rv2)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(rv2.Missing))
	assert.Equal(t, 1, len(rv2.Unsupported))
	assert.Equal(t, true, rv2.RebootRequired)
	fmt.Printf("Missing: %v\n", rv2.Missing)
	fmt.Printf("Unsupported: %v\n", rv2.Unsupported)

	_, err = ps.ParamSetSingle("param1", "another")
	assert.NoError(t, err)
	val, err := ps.ParamGetSingle("param1")
	assert.NoError(t, err)
	assert.Equal(t, "another", val)

	rv, err = ps.ParameterSetGet()
	assert.NotNil(t, rv)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0 (modified)", rv.Version)

	// force reload from nvs
	ps = nil
	ParameterSetsPurge()
	ps, err = NewParameterSet(nvs, defs)
	assert.NotNil(t, ps)
	assert.NoError(t, err)

	rv, err = ps.ParameterSetGet()
	assert.NotNil(t, rv)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0 (modified)", rv.Version)
	assert.Equal(t, 2, len(rv.Params))
	assert.Equal(t, "another", rv.Params["param1"])
	assert.Equal(t, "val23", rv.Params["param2"])
	assert.Equal(t, 0, len(rv.Missing))
	assert.Equal(t, 1, len(rv.Unsupported))
	assert.Equal(t, 0, len(rv.Invalid))

}
