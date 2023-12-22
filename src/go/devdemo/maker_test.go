package main

import (
	"testing"
)

func TestMaker(t *testing.T) {
	MakeWrapper("Lpcstub", "rpcprovider.go", "rpcwrapper.go")

}
