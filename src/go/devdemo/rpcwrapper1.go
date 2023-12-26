
package main

import (
	"encoding/json"
	"reflect"
)

type Lpcstub1 struct {
	client *Client
}

func NewLpcstub1(client *Client) *Lpcstub1 {
	return &Lpcstub1{client: client}
}


func (pthis *Lpcstub1) ToUpper(s string, a1 int, a2 int, a3 int) (string, error) {
    result := pthis.client.Invoke("ToUpper", s, a1, a2, a3)
    var err error
    var zero_0 string
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &[]interface{}{&zero_0})
    }
    return zero_0, err
}

func (pthis *Lpcstub1) Add(a int, b int) (int, error) {
    result := pthis.client.Invoke("Add", a, b)
    var err error
    var zero_0 int
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &[]interface{}{&zero_0})
    }
    return zero_0, err
}

func (pthis *Lpcstub1) Add2(a int, b int) (int, int, error) {
    result := pthis.client.Invoke("Add2", a, b)
    var err error
    var results []interface{}
    var zero_0 int
    var zero_1 int
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &results)
        if err == nil {
			zero_0 = pthis.client.ConvertToType(results[0], pthis.client.TypeNameToType("int")).(int)
			zero_1 = pthis.client.ConvertToType(results[1], pthis.client.TypeNameToType("int")).(int)
			
        }
    }
    return zero_0, zero_1, err
}

func (pthis *Lpcstub1) TestRPCFunc(arg1 int, arg2 float64, arg3 string, arg4 bool, arg5 []int, arg6 *string, arg7 CustomType, arg8 *CustomType) (int, *float64, string, bool, []int, *string, CustomType, *CustomType, error, error) {
    result := pthis.client.Invoke("TestRPCFunc", arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8)
    var err error
    var results []interface{}
    var zero_0 int
    var zero_1 *float64
    var zero_2 string
    var zero_3 bool
    var zero_4 []int
    var zero_5 *string
    var zero_6 CustomType
    var zero_7 *CustomType
    var zero_8 error
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &results)
        if err == nil {
			zero_0 = pthis.client.ConvertToType(results[0], pthis.client.TypeNameToType("int")).(int)
			zero_1 = pthis.client.ConvertToType(results[1], pthis.client.TypeNameToType("*float64")).(*float64)
			zero_2 = pthis.client.ConvertToType(results[2], pthis.client.TypeNameToType("string")).(string)
			zero_3 = pthis.client.ConvertToType(results[3], pthis.client.TypeNameToType("bool")).(bool)
			zero_4 = pthis.client.ConvertToType(results[4], pthis.client.TypeNameToType("[]int")).([]int)
			zero_5 = pthis.client.ConvertToType(results[5], pthis.client.TypeNameToType("*string")).(*string)
			zero_6 = pthis.client.ConvertToType(results[6], reflect.TypeOf(CustomType{})).(CustomType)
			zero_7 = pthis.client.ConvertToType(results[7], pthis.client.TypeNameToType("*CustomType")).(*CustomType)
			if results[8] != nil {
				zero_8 = pthis.client.ConvertToType(results[8], pthis.client.TypeNameToType("error")).(error)
			}
        }
    }
    return zero_0, zero_1, zero_2, zero_3, zero_4, zero_5, zero_6, zero_7, zero_8, err
}
