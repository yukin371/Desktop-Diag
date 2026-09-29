//go:build ignore

// stripcomments 打印一个 Go 源文件去除全部注释后的等价格式化结果。
//
// 用途：证明一次"只改注释"的重构确实没动代码——把新旧两个版本的输出
// 对比，完全相同即代码未变。基于 go/ast 而不是正则，因此字符串字面量里
// 的 //（例如 URL）不会被误删。
//
// 用法：go run tools/stripcomments/main.go <file.go>
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: stripcomments <file.go>")
		os.Exit(2)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 %s 失败: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	dropComments(file)

	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(os.Stdout, fset, file); err != nil {
		fmt.Fprintf(os.Stderr, "输出失败: %v\n", err)
		os.Exit(1)
	}
}

// dropComments 清空 AST 中所有注释挂载点。
//
// 必须逐个字段清，不能只把 File.Comments 置 nil：go/printer 打印声明时
// 读的是 Decl.Doc、Field.Doc 这些字段，而不是 File.Comments。
func dropComments(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.File:
			d.Doc, d.Comments = nil, nil
		case *ast.GenDecl:
			d.Doc = nil
		case *ast.FuncDecl:
			d.Doc = nil
		case *ast.TypeSpec:
			d.Doc, d.Comment = nil, nil
		case *ast.ValueSpec:
			d.Doc, d.Comment = nil, nil
		case *ast.Field:
			d.Doc, d.Comment = nil, nil
		case *ast.ImportSpec:
			d.Doc, d.Comment = nil, nil
		}
		return true
	})
}
