package main

import (
	"testing"
)

func TestMaker(t *testing.T) {
	MakeWrapper("Lpcstub1", "rpcprovider.go", "rpcwrapper.go")

}
