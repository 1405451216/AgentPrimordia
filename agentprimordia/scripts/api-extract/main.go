// api-extract 从 pkg/ 提取 Go 公共 API 签名，输出 JSON 契约文件。
//
// 使用 go/ast 解析所有 .go 文件（排除 _test.go），提取：
//   - 公开 type（struct/interface/alias）+ Stability 注解
//   - 公开 func（含参数和返回值签名）
//   - 公开 var/const
//
// 输出格式供跨语言 SDK 漂移检测使用。
//
// 用法：
//
//	go run ./scripts/api-extract/ -output ../sdk/typescript/api-contract.json
//	go run ./scripts/api-extract/ -no-timestamp   # 确定性输出（契约基线对比）
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// APIContract 顶层契约结构
type APIContract struct {
	Version     string      `json:"version"`
	GeneratedAt string      `json:"generated_at"`
	Modules     []ModuleAPI `json:"modules"`
}

// ModuleAPI 单个文件的 API 摘要
type ModuleAPI struct {
	Name      string     `json:"name"`
	File      string     `json:"file"`
	Stability string     `json:"stability,omitempty"`
	Types     []TypeAPI  `json:"types,omitempty"`
	Functions []FuncAPI  `json:"functions,omitempty"`
	Constants []ConstAPI `json:"constants,omitempty"`
	Variables []VarAPI   `json:"variables,omitempty"`
}

// TypeAPI 公开类型
type TypeAPI struct {
	Name      string     `json:"name"`
	Kind      string     `json:"kind"` // struct, interface, alias
	Stability string     `json:"stability,omitempty"`
	Fields    []FieldAPI `json:"fields,omitempty"`
	Methods   []string   `json:"methods,omitempty"`
	Doc       string     `json:"doc,omitempty"`
}

// FieldAPI 结构体字段
type FieldAPI struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

// FuncAPI 公开函数
type FuncAPI struct {
	Name      string   `json:"name"`
	Params    []string `json:"params,omitempty"`
	Returns   []string `json:"returns,omitempty"`
	Stability string   `json:"stability,omitempty"`
	Doc       string   `json:"doc,omitempty"`
}

// ConstAPI 公开常量
type ConstAPI struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	Stability string `json:"stability,omitempty"`
	Doc       string `json:"doc,omitempty"`
}

// VarAPI 公开变量
type VarAPI struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Stability string `json:"stability,omitempty"`
	Doc       string `json:"doc,omitempty"`
}

// stabilityRegexp 匹配 Stability 注解（如 "Stability: Stable" 或 "Stability: Experimental"）
var stabilityRegexp = regexp.MustCompile(`Stability:\s*(Stable|Experimental|Deprecated|Internal)`)

func main() {
	output := flag.String("output", "", "输出文件路径（默认 stdout）")
	pkgDir := flag.String("pkg", "pkg", "pkg 目录路径")
	noTimestamp := flag.Bool("no-timestamp", false, "确定性输出：generated_at 置空（用于契约基线对比）")
	flag.Parse()

	// 版本号从 pkg/agent.go 的 const Version 提取（单一事实来源），不再硬编码
	version, err := loadVersion(*pkgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 读取版本失败: %v\n", err)
		version = "unknown"
	}

	contract, err := extractAPI(*pkgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	contract.Version = version
	contract.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if *noTimestamp {
		contract.GeneratedAt = ""
	}

	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 序列化失败: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "创建目录失败: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*output, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "已写入 %s（%d 个模块）\n", *output, len(contract.Modules))
	} else {
		fmt.Println(string(data))
	}
}

// extractAPI 解析 pkg/ 目录下所有 .go 文件，提取公共 API
func extractAPI(pkgDir string) (*APIContract, error) {
	fset := token.NewFileSet()

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录 %s: %w", pkgDir, err)
	}

	var modules []ModuleAPI

	for _, entry := range entries {
		if entry.IsDir() {
			// 递归处理子目录（如 pkg/logger/）
			subDir := filepath.Join(pkgDir, entry.Name())
			subContract, err := extractAPI(subDir)
			if err != nil {
				return nil, err
			}
			modules = append(modules, subContract.Modules...)
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		filePath := filepath.Join(pkgDir, name)
		mod, err := parseFile(fset, filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 解析 %s 失败: %v\n", filePath, err)
			continue
		}
		if mod != nil {
			modules = append(modules, *mod)
		}
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].File < modules[j].File
	})

	return &APIContract{
		Modules: modules,
	}, nil
}

// loadVersion 从 pkg/agent.go 的 `const Version = "x.y.z"` 提取版本号。
// 版本号以 pkg/agent.go 为单一事实来源，避免工具链内多处硬编码漂移。
func loadVersion(pkgDir string) (string, error) {
	path := filepath.Join(pkgDir, "agent.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return "", fmt.Errorf("解析 %s: %w", path, err)
	}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name == "Version" && i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						return strings.Trim(lit.Value, `"`), nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("%s 中未找到 const Version", path)
}

// parseFile 解析单个 Go 文件，提取公开 API 签名
func parseFile(fset *token.FileSet, path string) (*ModuleAPI, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// 模块名取自文件名（去掉 .go 后缀）
	baseName := filepath.Base(path)
	moduleName := strings.TrimSuffix(baseName, ".go")

	mod := &ModuleAPI{
		Name: moduleName,
		File: filepath.ToSlash(path),
	}

	// 提取文件级 Stability 注解
	if file.Doc != nil {
		if m := stabilityRegexp.FindStringSubmatch(file.Doc.Text()); m != nil {
			mod.Stability = m[1]
		}
	}

	// 收集方法接收器（用于关联到类型）
	methodsByType := make(map[string][]string)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				// 方法：收集到对应类型
				typeName := receiverTypeName(d.Recv)
				if typeName != "" && d.Name.IsExported() {
					methodsByType[typeName] = append(methodsByType[typeName], d.Name.Name)
				}
			} else if d.Name.IsExported() {
				// 公开函数
				mod.Functions = append(mod.Functions, FuncAPI{
					Name:      d.Name.Name,
					Params:    formatFieldList(d.Type.Params),
					Returns:   formatFieldList(d.Type.Results),
					Stability: extractStability(d.Doc),
					Doc:       docSummary(d.Doc),
				})
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					// 优先使用类型自身的注释，否则回退到 GenDecl 的注释
					doc := s.Doc
					if doc == nil {
						doc = d.Doc
					}
					t := TypeAPI{
						Name:      s.Name.Name,
						Stability: extractStability(doc),
						Doc:       docSummary(doc),
					}
					switch st := s.Type.(type) {
					case *ast.StructType:
						t.Kind = "struct"
						t.Fields = extractFields(st)
					case *ast.InterfaceType:
						t.Kind = "interface"
						t.Methods = extractInterfaceMethods(st)
					default:
						t.Kind = "alias"
					}
					mod.Types = append(mod.Types, t)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						if !name.IsExported() {
							continue
						}
						// 优先使用 ValueSpec 自身的注释
						doc := s.Doc
						if doc == nil {
							doc = d.Doc
						}
						stability := extractStability(doc)
						if d.Tok == token.CONST {
							mod.Constants = append(mod.Constants, ConstAPI{
								Name:      name.Name,
								Value:     valueString(s.Values),
								Stability: stability,
								Doc:       docSummary(doc),
							})
						} else {
							mod.Variables = append(mod.Variables, VarAPI{
								Name:      name.Name,
								Type:      typeString(s.Type),
								Stability: stability,
								Doc:       docSummary(doc),
							})
						}
					}
				}
			}
		}
	}

	// 关联方法到类型
	for i := range mod.Types {
		if methods, ok := methodsByType[mod.Types[i].Name]; ok {
			mod.Types[i].Methods = methods
		}
	}

	// 跳过空模块
	if len(mod.Types) == 0 && len(mod.Functions) == 0 &&
		len(mod.Constants) == 0 && len(mod.Variables) == 0 {
		return nil, nil
	}

	return mod, nil
}

// ===== 辅助函数 =====

// extractStability 从注释组中提取 Stability 注解，返回空字符串表示无注解
func extractStability(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	if m := stabilityRegexp.FindStringSubmatch(doc.Text()); m != nil {
		return m[1]
	}
	return ""
}

// receiverTypeName 提取方法接收器的类型名
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// formatFieldList 将函数字段列表格式化为字符串切片
func formatFieldList(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var result []string
	for _, field := range fl.List {
		ts := typeString(field.Type)
		if len(field.Names) == 0 {
			result = append(result, ts)
		} else {
			for _, name := range field.Names {
				result = append(result, name.Name+" "+ts)
			}
		}
	}
	return result
}

// extractFields 提取结构体的公开字段
func extractFields(st *ast.StructType) []FieldAPI {
	var fields []FieldAPI
	if st.Fields == nil {
		return fields
	}
	for _, field := range st.Fields.List {
		ts := typeString(field.Type)
		tag := ""
		if field.Tag != nil {
			tag = field.Tag.Value
		}
		if len(field.Names) == 0 {
			// 嵌入字段
			fields = append(fields, FieldAPI{Name: ts, Type: ts, Tag: tag})
		} else {
			for _, name := range field.Names {
				if name.IsExported() {
					fields = append(fields, FieldAPI{Name: name.Name, Type: ts, Tag: tag})
				}
			}
		}
	}
	return fields
}

// extractInterfaceMethods 提取接口的公开方法名
func extractInterfaceMethods(it *ast.InterfaceType) []string {
	var methods []string
	if it.Methods == nil {
		return methods
	}
	for _, m := range it.Methods.List {
		for _, name := range m.Names {
			if name.IsExported() {
				methods = append(methods, name.Name)
			}
		}
	}
	return methods
}

// typeString 将 AST 表达式转换为可读的类型字符串
func typeString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[...]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + typeString(t.Value)
		case ast.RECV:
			return "<-chan " + typeString(t.Value)
		default:
			return "chan " + typeString(t.Value)
		}
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// valueString 提取常量/变量的值字符串表示
func valueString(values []ast.Expr) string {
	if len(values) == 0 {
		return ""
	}
	switch v := values[0].(type) {
	case *ast.BasicLit:
		return v.Value
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return typeString(v.X) + "." + v.Sel.Name
	case *ast.UnaryExpr:
		return v.Op.String() + valueString([]ast.Expr{v.X})
	case *ast.CallExpr:
		return typeString(v.Fun) + "(...)"
	default:
		return ""
	}
}

// docSummary 提取注释的第一行作为摘要，限制 120 字符
func docSummary(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	text := strings.TrimSpace(doc.Text())
	// 只取第一行作为摘要
	if idx := strings.Index(text, "\n"); idx > 0 {
		text = text[:idx]
	}
	// 限制长度
	if len(text) > 120 {
		text = text[:117] + "..."
	}
	return text
}
