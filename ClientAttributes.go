package uwebsocket

import (
	"fmt"

	"github.com/dunv/uhelpers"
)

type ClientAttributes map[string]interface{}

func (c ClientAttributes) IsFlagSet(key string) bool {
	rawMap := map[string]interface{}(c)
	return uhelpers.IsMatchingBoolPointerInMap(uhelpers.PtrToBool(true), rawMap, key)
}

func (c ClientAttributes) HasMatch(key string, value string) bool {
	rawMap := map[string]interface{}(c)
	fmt.Println("rawMap", rawMap)
	fmt.Println("ptr", uhelpers.PtrToString(value))
	fmt.Println("checking key", key)
	fmt.Println("checking value", value)
	val := uhelpers.IsMatchingStringPointerInMap(uhelpers.PtrToString(value), rawMap, key)
	fmt.Println("checking result", val)
	return val
}

type ClientMessage struct {
	Message []byte
	Client  ClientAttributes
}
