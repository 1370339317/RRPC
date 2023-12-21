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
const tmpl = `func (c *Client) {{.Name}}({{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}} {{$param.Type}}{{end}}) ({{range $index, $ret := .Results}}{{if $index}}, {{end}}{{$ret}}{{end}}, error) {
    result := c.Invoke("{{.Name}}", {{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}}{{end}})
    if result.Err != nil {
        {{range $index, $ret := .Results}}var zero_{{$index}} {{$ret}}
        switch v := zeroValue("{{$ret}}").(type) {
        case {{$ret}}:
            zero_{{$index}} = v
        }
        {{end}}
        return {{range $index, $ret := .Results}}{{if $index}}, {{end}}zero_{{$index}}{{end}}, result.Err
    }
    {{if (index .Results 0)}}return {{range $index, $ret := .Results}}{{if $index}}, {{end}}result.Result[{{$index}}].({{$ret}}){{end}}, nil{{else}}return nil, nil{{end}}
}
`

// 解析函数参数和返回值
func parseFuncDecl(f *ast.FuncDecl, out *os.File) {
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
			paramName := typeNameBuf.String()
			if len(p.Names) > 0 {
				for _, n := range p.Names {
					params = append(params, struct {
						Name string
						Type string
					}{Name: n.Name, Type: paramName})
				}
			} else {
				params = append(params, struct {
					Name string
					Type string
				}{Type: paramName})
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
			resultName := typeNameBuf.String()
			results = append(results, resultName)
		}
	}

	// 使用模板生成函数代码
	t := template.Must(template.New("func").Parse(tmpl))
	err := t.Execute(out, struct {
		Name   string
		Params []struct {
			Name string
			Type string
		}
		Results []string
	}{
		Name:    name,
		Params:  params,
		Results: results,
	})
	if err != nil {
		log.Fatal(err)
	}
}

const predefCode = `
import (
	"reflect"
	"strings"
)

func zeroValue(typeName string) interface{} {
	typeName = strings.TrimPrefix(typeName, "*")
	t := reflect.TypeOf(typeName)
	if t == nil {
		// 如果类型不存在，返回nil
		return nil
	}
	return reflect.Zero(t).Interface()
}
`

func MakeWrapper(filename string, outFilename string) {
	// 创建输出文件
	out, err := os.Create(outFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	out.WriteString(predefCode)

	// 解析文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	// 遍历 AST 节点
	for _, f := range node.Decls {
		if fn, isFn := f.(*ast.FuncDecl); isFn {
			parseFuncDecl(fn, out)
		}
	}
}
