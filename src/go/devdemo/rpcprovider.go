// rpc_functions.go
package main

import (
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
