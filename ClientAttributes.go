package uwebsocket

import (
	"github.com/dunv/uhelpers"
)

type ClientAttributes map[string]interface{}

func (c ClientAttributes) IsFlagSet(key string) bool {
	rawMap := map[string]interface{}(c)
	return uhelpers.IsMatchingBoolPointerInMap(uhelpers.PtrToBool(true), rawMap, key)
}

func (c ClientAttributes) HasMatch(key string, value string) bool {
	rawMap := map[string]interface{}(c)
	val := uhelpers.IsMatchingStringPointerInMap(uhelpers.PtrToString(value), rawMap, key)
	return val
}

type ClientMessage struct {
	Message []byte
	Client  ClientAttributes
}
