package uwebsocket

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/dunv/uhelpers"
)

var (
	ErrKeyDoesNotExist = errors.New("key does not exist")
	ErrKeyHasWrongType = errors.New("key has wrong type")
)

// Deprecated: use ErrKeyDoesNotExist.
var KEY_DOES_NOT_EXIST_ERR = ErrKeyDoesNotExist //nolint:staticcheck // Retained for source compatibility.

// Deprecated: use ErrKeyHasWrongType.
var KEY_HAS_WRONG_TYPE_ERR = ErrKeyHasWrongType //nolint:staticcheck // Retained for source compatibility.

type ClientAttributes struct {
	attrs map[string]interface{}
}

func NewClientAttributes() *ClientAttributes {
	return &ClientAttributes{
		attrs: map[string]interface{}{},
	}
}

func (c *ClientAttributes) SetString(key string, value string) *ClientAttributes {
	c.attrs[key] = uhelpers.Ptr(value)
	return c
}

func (c *ClientAttributes) GetString(key string) (string, error) {
	if val, ok := c.attrs[key]; ok {
		if typed, ok := val.(*string); ok {
			return *typed, nil
		}
		return "", ErrKeyHasWrongType
	}
	return "", ErrKeyDoesNotExist
}

func (c *ClientAttributes) SetBool(key string, value bool) *ClientAttributes {
	c.attrs[key] = uhelpers.Ptr(value)
	return c
}

func (c *ClientAttributes) GetBool(key string) (bool, error) {
	if val, ok := c.attrs[key]; ok {
		if typed, ok := val.(*bool); ok {
			return *typed, nil
		}
		return false, ErrKeyHasWrongType
	}
	return false, ErrKeyDoesNotExist
}

func (c *ClientAttributes) IsFlagSet(key string) bool {
	return uhelpers.IsMatchingBoolPointerInMap(uhelpers.Ptr(true), c.attrs, key)
}

func (c *ClientAttributes) HasMatch(key string, value string) bool {
	val := uhelpers.IsMatchingStringPointerInMap(uhelpers.Ptr(value), c.attrs, key)
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
			log.Printf("uwebsocket.ClientAttributes: unknown type %T\n", typed)
		}
	}
	return strings.Join(out, ", ")
}
