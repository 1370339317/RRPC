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

// 函数模板
// 函数模板
const tmpl = `func (pthis *{{.ClassName}}) {{.Name}}({{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}} {{$param.Type}}{{end}}) ({{range $index, $ret := .Results}}{{if $index}}, {{end}}{{$ret}}{{end}}, error) {
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
            {{range $index, $ret := .Results}}zero_{{$index}} = convertType(results[{{$index}}], "{{$ret}}")
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
	"strings"
)

type {{.ClassName}} struct {
	client *Client
}

func New{{.ClassName}}(client *Client) *{{.ClassName}} {
	return &{{.ClassName}}{client: client}
}

var zeroValues = map[string]interface{}{
	"string": "",
	"int":    0,
	// Add other types as needed
}

func (pthis *{{.ClassName}}) zeroValue(typeName string) interface{} {
	typeName = strings.TrimPrefix(typeName, "*")
	return zeroValues[typeName]
}
func convertType(val interface{}, targetType string) interface{} {
    switch targetType {
    case "int":
        if v, ok := val.(float64); ok {
            return int(v)
        }
    case "int64":
        if v, ok := val.(float64); ok {
            return int64(v)
        }
    // Add other type conversions as needed
    }
    return val
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
