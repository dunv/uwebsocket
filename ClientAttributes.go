package uwebsocket

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dunv/uhelpers"
	"github.com/dunv/ulog"
)

var KEY_DOES_NOT_EXIST_ERR = errors.New("key does not exist")
var KEY_HAS_WRONG_TYPE_ERR = errors.New("key has wrong type")

type ClientAttributes struct {
	attrs map[string]interface{}
}

func NewClientAttributes() *ClientAttributes {
	return &ClientAttributes{
		attrs: map[string]interface{}{},
	}
}

func (c *ClientAttributes) SetString(key string, value string) *ClientAttributes {
	c.attrs[key] = uhelpers.PtrToString(value)
	return c
}

func (c *ClientAttributes) GetString(key string) (string, error) {
	if val, ok := c.attrs[key]; ok {
		if typed, ok := val.(*string); ok {
			return *typed, nil
		}
		return "", KEY_HAS_WRONG_TYPE_ERR
	}
	return "", KEY_DOES_NOT_EXIST_ERR
}

func (c *ClientAttributes) SetBool(key string, value bool) *ClientAttributes {
	c.attrs[key] = uhelpers.PtrToBool(value)
	return c
}

func (c *ClientAttributes) GetBool(key string) (bool, error) {
	if val, ok := c.attrs[key]; ok {
		if typed, ok := val.(*bool); ok {
			return *typed, nil
		}
		return false, KEY_HAS_WRONG_TYPE_ERR
	}
	return false, KEY_DOES_NOT_EXIST_ERR
}

func (c *ClientAttributes) IsFlagSet(key string) bool {
	return uhelpers.IsMatchingBoolPointerInMap(uhelpers.PtrToBool(true), c.attrs, key)
}

func (c *ClientAttributes) HasMatch(key string, value string) bool {
	val := uhelpers.IsMatchingStringPointerInMap(uhelpers.PtrToString(value), c.attrs, key)
	return val
}

func (c *ClientAttributes) String() string {
	out := []string{}
	for k, v := range c.attrs {
		switch typed := v.(type) {
		case *string:
			out = append(out, fmt.Sprintf("%s: %s", k, *typed))
		case *bool:
			out = append(out, fmt.Sprintf("%s: %t", k, *typed))
		default:
			ulog.Warnf("")
		}
	}
	return strings.Join(out, ", ")
}
