package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"strings"
	"text/template"
)

const tmpl = `
func (pthis *{{.ClassName}}) {{.Name}}({{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}} {{$param.Type}}{{end}}) ({{range $index, $ret := .Results}}{{if $index}}, {{end}}{{$ret}}{{end}}, error) {
    result := pthis.client.Invoke("{{.Name}}", {{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}}{{end}})
    var err error
    {{if eq (len .Results) 1}} // 如果只有一个返回值
    var zero_0 {{index .Results 0}}
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &[]interface{}{&zero_0})
    }
    {{else}} // 如果有多个返回值
    var results []interface{}
    {{range $index, $ret := .Results}}var zero_{{$index}} {{$ret}}
    {{end}}
    if result.Err != nil {
        err = result.Err
    } else {
        err = json.Unmarshal([]byte(result.Result), &results)
        if err == nil {
			{{range $index, $ret := .Results}}zero_{{$index}} = convertToType(results[{{$index}}], "{{$ret}}").({{$ret}})
			{{end}}
        }
    }
    {{end}}
    return {{range $index, $ret := .Results}}{{if $index}}, {{end}}zero_{{$index}}{{end}}, err
}
`

const predefCode = `
package main

import (
	"encoding/json"
	"reflect"
	"strings"
)

type {{.ClassName}} struct {
	client *Client
}

func New{{.ClassName}}(client *Client) *{{.ClassName}} {
	return &{{.ClassName}}{client: client}
}

func convertToType(val interface{}, typeName string) interface{} {
	switch {
	case strings.HasPrefix(typeName, "*"): // Handle pointers
		elemType := typeName[1:]
		elemVal := convertToType(val, elemType)
		ptr := reflect.New(reflect.TypeOf(elemVal))
		ptr.Elem().Set(reflect.ValueOf(elemVal))
		return ptr.Interface()
	case strings.HasPrefix(typeName, "[]"): // Handle slices
		elemType := typeName[2:]
		if v, ok := val.([]interface{}); ok {
			slice := reflect.MakeSlice(reflect.SliceOf(typeNameToType(elemType)), len(v), len(v))
			for i, elem := range v {
				slice.Index(i).Set(reflect.ValueOf(convertToType(elem, elemType)))
			}
			return slice.Interface()
		}
	case strings.HasPrefix(typeName, "map["): // Handle maps
		keyType := typeName[4:strings.Index(typeName, "]")]
		valueType := typeName[strings.Index(typeName, "]")+1:]
		if v, ok := val.(map[string]interface{}); ok {
			mapType := reflect.MapOf(typeNameToType(keyType), typeNameToType(valueType))
			m := reflect.MakeMap(mapType)
			for k, elem := range v {
				m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(convertToType(elem, valueType)))
			}
			return m.Interface()
		}
	default: // Handle basic types
		return convertBasicType(val, typeName)
	}
	return nil
}

func typeNameToType(typeName string) reflect.Type {
	switch typeName {
	case "int":
		return reflect.TypeOf(int(0))
	case "int64":
		return reflect.TypeOf(int64(0))
	case "float64":
		return reflect.TypeOf(float64(0))
	case "string":
		return reflect.TypeOf("")
	case "bool":
		return reflect.TypeOf(true)
	default:
		panic("unsupported type: " + typeName)
	}
}

func convertBasicType(val interface{}, typeName string) interface{} {
	switch typeName {
	case "int":
		if v, ok := val.(float64); ok {
			return int(v)
		}
	case "int64":
		if v, ok := val.(float64); ok {
			return int64(v)
		}
	case "float64":
		if v, ok := val.(float64); ok {
			return v
		}
	case "string":
		if v, ok := val.(string); ok {
			return v
		}
	case "bool":
		if v, ok := val.(bool); ok {
			return v
		}
	default:
		panic("unsupported type: " + typeName)
	}
	return nil
}
`

func MakeWrapper(className, filename, outFilename string) {
	// 创建输出文件
	out, err := os.Create(outFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// 添加类定义和 zeroValue 方法
	t := template.Must(template.New("predef").Parse(predefCode))
	err = t.Execute(out, struct {
		ClassName string
	}{
		ClassName: className,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 解析文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	// 遍历 AST 节点
	for _, f := range node.Decls {
		if fn, isFn := f.(*ast.FuncDecl); isFn {
			parseFuncDecl(className, fn, out)
		}
	}
}

func parseFuncDecl(className string, f *ast.FuncDecl, out *os.File) {
	// 函数名
	name := f.Name.Name

	// 参数
	var params []struct {
		Name string
		Type string
	}

	if f.Type.Params != nil {
		for _, p := range f.Type.Params.List {
			typeNameBuf := &strings.Builder{}
			if err := printer.Fprint(typeNameBuf, token.NewFileSet(), p.Type); err != nil {
				log.Fatal(err)
			}
			paramType := typeNameBuf.String()
			if len(p.Names) > 0 {
				for _, n := range p.Names {
					params = append(params, struct {
						Name string
						Type string
					}{n.Name, paramType})
				}
			} else {
				params = append(params, struct {
					Name string
					Type string
				}{"", paramType})
			}
		}
	}

	// 返回值
	var results []string
	if f.Type.Results != nil {
		for _, r := range f.Type.Results.List {
			typeNameBuf := &strings.Builder{}
			if err := printer.Fprint(typeNameBuf, token.NewFileSet(), r.Type); err != nil {
				log.Fatal(err)
			}
			results = append(results, typeNameBuf.String())
		}
	}

	// 使用模板生成函数代码
	t := template.Must(template.New("func").Parse(tmpl))
	err := t.Execute(out, struct {
		ClassName string
		Name      string
		Params    []struct {
			Name string
			Type string
		}
		Results []string
	}{
		ClassName: className,
		Name:      name,
		Params:    params,
		Results:   results,
	})
	if err != nil {
		log.Fatal(err)
	}
}
