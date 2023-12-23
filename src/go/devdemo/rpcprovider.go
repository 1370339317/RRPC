// rpc_functions.go
package main

import (
	"errors"
	"fmt"
	"strings"
)

func ToUpper(s string, a1, a2, a3 int) string {
	fmt.Printf("rpc过程 ToUpper被调用了,参数:%s ,%d,%d,%d\r\n", s, a1, a2, a3)
	// 函数实现...
	return strings.ToUpper(s)
}

func Add(a, b int) int {
	fmt.Printf("rpc过程 Add被调用了,参数:%d,%d\r\n", a, b)
	// 函数实现...
	return a + b
}
func Add2(a, b int) (int, int) {
	fmt.Printf("rpc过程 Add2 被调用了,参数:%d,%d\r\n", a, b)
	// 函数实现...
	return a + b, a
}

type CustomType struct {
	Field1 int
	Field2 string
}

func TestRPCFunc(arg1 int, arg2 float64, arg3 string, arg4 bool, arg5 []int, arg6 *string, arg7 CustomType) (int, *float64, string, bool, []int, *string, CustomType, error) {
	if arg1 < 0 {
		return 0, nil, "", false, nil, nil, CustomType{}, errors.New("arg1 cannot be negative")
	}
	if arg2 < 0 {
		return 0, nil, "", false, nil, nil, CustomType{}, errors.New("arg2 cannot be negative")
	}
	if arg3 == "" {
		return 0, nil, "", false, nil, nil, CustomType{}, errors.New("arg3 cannot be empty")
	}
	if len(arg5) == 0 {
		return 0, nil, "", false, nil, nil, CustomType{}, errors.New("arg5 cannot be empty")
	}
	if arg6 == nil {
		return 0, nil, "", false, nil, nil, CustomType{}, errors.New("arg6 cannot be nil")
	}

	res1 := arg1 * 2
	res2 := arg2 * 2
	res3 := arg3 + arg3
	res4 := !arg4
	res5 := append(arg5, arg5...)
	res6 := *arg6 + *arg6
	res7 := CustomType{
		Field1: arg7.Field1 * 2,
		Field2: arg7.Field2 + arg7.Field2,
	}

	return res1, &res2, res3, res4, res5, &res6, res7, nil
}
