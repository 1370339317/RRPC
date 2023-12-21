// rpc_functions.go
package main

import "strings"

func ToUpper(s string, a1, a2, a3 int) string {
	// 函数实现...
	return strings.ToUpper(s)
}

func Add(a, b int) int {
	// 函数实现...
	return a + b
}
func Add2(a, b int) (int, int) {
	// 函数实现...
	return a + b, a
}
