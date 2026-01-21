package codebaseindex

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"regexp"
	"strings"
)

// extractSymbols extracts symbols from all candidate files
func (m *Manager) extractSymbols(candidates []*FileEntry) error {
	for _, entry := range candidates {
		path := entry.Path
		if !strings.HasPrefix(path, "/") {
			path = m.config.Root + "/" + path
		}

		content, err := readFilePartial(path, maxSymbolReadSize)
		if err != nil {
			continue // Skip files we can't read
		}

		// Find the appropriate adapter
		for _, adapter := range m.adapters {
			if adapter.Name() == entry.Language {
				symbols, err := adapter.ExtractSymbols(entry.Path, content)
				if err == nil {
					entry.Symbols = symbols
				}
				break
			}
		}
	}

	return nil
}

// readFilePartial reads up to maxBytes from a file
func readFilePartial(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(io.LimitReader(f, maxBytes))
}

// extractGoSymbols extracts symbols from Go source code using AST
func extractGoSymbols(path string, content []byte) (*SymbolInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		// Fall back to regex if parsing fails
		return extractGoSymbolsRegex(content)
	}

	info := &SymbolInfo{
		Package: file.Name.Name,
	}

	// Extract imports
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		info.Imports = append(info.Imports, importPath)
	}

	// Extract declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			fn := FunctionInfo{
				Name:       d.Name.Name,
				IsExported: ast.IsExported(d.Name.Name),
			}

			// Check if it's a method
			if d.Recv != nil && len(d.Recv.List) > 0 {
				fn.IsMethod = true
				if t, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := t.X.(*ast.Ident); ok {
						fn.Receiver = ident.Name
					}
				} else if ident, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					fn.Receiver = ident.Name
				}
			}

			// Build signature
			fn.Signature = buildGoFuncSignature(d)

			info.Functions = append(info.Functions, fn)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					ti := TypeInfo{
						Name:       s.Name.Name,
						IsExported: ast.IsExported(s.Name.Name),
					}

					switch t := s.Type.(type) {
					case *ast.StructType:
						ti.Kind = "struct"
						if t.Fields != nil {
							for _, field := range t.Fields.List {
								for _, name := range field.Names {
									ti.Fields = append(ti.Fields, name.Name)
								}
							}
						}
					case *ast.InterfaceType:
						ti.Kind = "interface"
						if t.Methods != nil {
							for _, method := range t.Methods.List {
								for _, name := range method.Names {
									ti.Methods = append(ti.Methods, name.Name)
								}
							}
						}
					default:
						ti.Kind = "type"
					}

					info.Types = append(info.Types, ti)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						if d.Tok == token.CONST {
							info.Constants = append(info.Constants, name.Name)
						} else {
							info.Variables = append(info.Variables, name.Name)
						}
					}
				}
			}
		}
	}

	// Detect frameworks and risk tags
	info.Frameworks = detectGoFrameworks(info.Imports)
	info.RiskTags = detectGoRiskTags(content, info.Imports)

	return info, nil
}

// buildGoFuncSignature builds a function signature string
func buildGoFuncSignature(d *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")

	if d.Recv != nil && len(d.Recv.List) > 0 {
		sb.WriteString("(")
		if t, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
			if ident, ok := t.X.(*ast.Ident); ok {
				sb.WriteString("*")
				sb.WriteString(ident.Name)
			}
		} else if ident, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
			sb.WriteString(ident.Name)
		}
		sb.WriteString(") ")
	}

	sb.WriteString(d.Name.Name)
	sb.WriteString("()")

	return sb.String()
}

// extractGoSymbolsRegex is a fallback when AST parsing fails
func extractGoSymbolsRegex(content []byte) (*SymbolInfo, error) {
	info := &SymbolInfo{}
	text := string(content)

	// Package
	if match := regexp.MustCompile(`package\s+(\w+)`).FindStringSubmatch(text); len(match) > 1 {
		info.Package = match[1]
	}

	// Functions
	funcRe := regexp.MustCompile(`func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	for _, match := range funcRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Functions = append(info.Functions, FunctionInfo{
				Name:       match[1],
				IsExported: isUpperCase(match[1][0]),
			})
		}
	}

	// Types
	typeRe := regexp.MustCompile(`type\s+(\w+)\s+(struct|interface)`)
	for _, match := range typeRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 2 {
			info.Types = append(info.Types, TypeInfo{
				Name:       match[1],
				Kind:       match[2],
				IsExported: isUpperCase(match[1][0]),
			})
		}
	}

	return info, nil
}

// detectGoFrameworks detects common Go frameworks from imports
func detectGoFrameworks(imports []string) []string {
	var frameworks []string
	frameworkMap := map[string]string{
		"github.com/gin-gonic/gin":      "gin",
		"github.com/labstack/echo":      "echo",
		"github.com/gofiber/fiber":      "fiber",
		"github.com/gorilla/mux":        "gorilla",
		"net/http":                      "stdlib-http",
		"github.com/spf13/cobra":        "cobra",
		"github.com/urfave/cli":         "cli",
		"google.golang.org/grpc":        "grpc",
		"github.com/graphql-go/graphql": "graphql",
		"gorm.io/gorm":                  "gorm",
		"github.com/jmoiron/sqlx":       "sqlx",
	}

	for _, imp := range imports {
		for pattern, name := range frameworkMap {
			if strings.HasPrefix(imp, pattern) {
				frameworks = append(frameworks, name)
				break
			}
		}
	}

	return frameworks
}

// detectGoRiskTags detects risk areas in Go code
func detectGoRiskTags(content []byte, imports []string) []string {
	var tags []string
	text := string(content)

	// Check imports for risk indicators
	riskImports := map[string]string{
		"crypto":   "crypto",
		"database": "database",
		"sql":      "database",
		"sync":     "concurrency",
		"context":  "concurrency",
		"auth":     "auth",
		"oauth":    "auth",
		"jwt":      "auth",
		"bcrypt":   "auth",
		"unsafe":   "unsafe",
		"reflect":  "reflection",
		"cgo":      "cgo",
	}

	for _, imp := range imports {
		for pattern, tag := range riskImports {
			if strings.Contains(strings.ToLower(imp), pattern) {
				tags = appendUnique(tags, tag)
			}
		}
	}

	// Check code patterns
	if regexp.MustCompile(`go\s+\w+\(`).MatchString(text) {
		tags = appendUnique(tags, "concurrency")
	}
	if regexp.MustCompile(`chan\s+`).MatchString(text) {
		tags = appendUnique(tags, "concurrency")
	}
	if regexp.MustCompile(`password|secret|token|apikey|api_key`).MatchString(strings.ToLower(text)) {
		tags = appendUnique(tags, "secrets")
	}

	return tags
}

// extractPythonSymbols extracts symbols from Python source code using regex
func extractPythonSymbols(path string, content []byte) (*SymbolInfo, error) {
	info := &SymbolInfo{}
	text := string(content)

	// Imports
	importRe := regexp.MustCompile(`(?m)^(?:from\s+(\S+)\s+)?import\s+(.+)$`)
	for _, match := range importRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && match[1] != "" {
			info.Imports = append(info.Imports, match[1])
		} else if len(match) > 2 {
			info.Imports = append(info.Imports, strings.TrimSpace(match[2]))
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^class\s+(\w+)(?:\([^)]*\))?:`)
	for _, match := range classRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Types = append(info.Types, TypeInfo{
				Name:       match[1],
				Kind:       "class",
				IsExported: !strings.HasPrefix(match[1], "_"),
			})
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:async\s+)?def\s+(\w+)\s*\(`)
	for _, match := range funcRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Functions = append(info.Functions, FunctionInfo{
				Name:       match[1],
				IsExported: !strings.HasPrefix(match[1], "_"),
			})
		}
	}

	// Detect frameworks
	info.Frameworks = detectPythonFrameworks(info.Imports)
	info.RiskTags = detectPythonRiskTags(text, info.Imports)

	return info, nil
}

// detectPythonFrameworks detects common Python frameworks
func detectPythonFrameworks(imports []string) []string {
	var frameworks []string
	frameworkMap := map[string]string{
		"flask":      "flask",
		"django":     "django",
		"fastapi":    "fastapi",
		"starlette":  "starlette",
		"tornado":    "tornado",
		"aiohttp":    "aiohttp",
		"sqlalchemy": "sqlalchemy",
		"pandas":     "pandas",
		"numpy":      "numpy",
		"tensorflow": "tensorflow",
		"torch":      "pytorch",
		"pytest":     "pytest",
		"unittest":   "unittest",
		"celery":     "celery",
	}

	for _, imp := range imports {
		for pattern, name := range frameworkMap {
			if strings.Contains(strings.ToLower(imp), pattern) {
				frameworks = appendUnique(frameworks, name)
			}
		}
	}

	return frameworks
}

// detectPythonRiskTags detects risk areas in Python code
func detectPythonRiskTags(text string, imports []string) []string {
	var tags []string

	riskPatterns := map[string]string{
		"crypto":          "crypto",
		"hashlib":         "crypto",
		"bcrypt":          "auth",
		"jwt":             "auth",
		"oauth":           "auth",
		"sql":             "database",
		"asyncio":         "concurrency",
		"threading":       "concurrency",
		"multiprocessing": "concurrency",
		"subprocess":      "subprocess",
		"os.system":       "subprocess",
		"eval(":           "code-execution",
		"exec(":           "code-execution",
	}

	for _, imp := range imports {
		for pattern, tag := range riskPatterns {
			if strings.Contains(strings.ToLower(imp), pattern) {
				tags = appendUnique(tags, tag)
			}
		}
	}

	for pattern, tag := range riskPatterns {
		if strings.Contains(strings.ToLower(text), pattern) {
			tags = appendUnique(tags, tag)
		}
	}

	return tags
}

// extractTypeScriptSymbols extracts symbols from TypeScript/JavaScript code
func extractTypeScriptSymbols(path string, content []byte) (*SymbolInfo, error) {
	info := &SymbolInfo{}
	text := string(content)

	// Imports
	importRe := regexp.MustCompile(`(?m)^import\s+.*?from\s+['"]([^'"]+)['"]`)
	for _, match := range importRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Imports = append(info.Imports, match[1])
		}
	}

	// Require statements
	requireRe := regexp.MustCompile(`require\(['"]([^'"]+)['"]\)`)
	for _, match := range requireRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Imports = appendUnique(info.Imports, match[1])
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`)
	for _, match := range classRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Types = append(info.Types, TypeInfo{
				Name:       match[1],
				Kind:       "class",
				IsExported: strings.Contains(match[0], "export"),
			})
		}
	}

	// Interfaces
	interfaceRe := regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+(\w+)`)
	for _, match := range interfaceRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Types = append(info.Types, TypeInfo{
				Name:       match[1],
				Kind:       "interface",
				IsExported: strings.Contains(match[0], "export"),
			})
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	for _, match := range funcRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Functions = append(info.Functions, FunctionInfo{
				Name:       match[1],
				IsExported: strings.Contains(match[0], "export"),
			})
		}
	}

	// Arrow functions with export
	arrowRe := regexp.MustCompile(`(?m)^export\s+(?:const|let)\s+(\w+)\s*=\s*(?:async\s+)?\(`)
	for _, match := range arrowRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Functions = append(info.Functions, FunctionInfo{
				Name:       match[1],
				IsExported: true,
			})
		}
	}

	// Detect frameworks
	info.Frameworks = detectTypeScriptFrameworks(info.Imports)
	info.RiskTags = detectTypeScriptRiskTags(text, info.Imports)

	return info, nil
}

// detectTypeScriptFrameworks detects common TypeScript/JS frameworks
func detectTypeScriptFrameworks(imports []string) []string {
	var frameworks []string
	frameworkMap := map[string]string{
		"react":    "react",
		"vue":      "vue",
		"angular":  "angular",
		"svelte":   "svelte",
		"next":     "nextjs",
		"express":  "express",
		"fastify":  "fastify",
		"nest":     "nestjs",
		"prisma":   "prisma",
		"typeorm":  "typeorm",
		"mongoose": "mongoose",
		"jest":     "jest",
		"mocha":    "mocha",
		"graphql":  "graphql",
		"apollo":   "apollo",
	}

	for _, imp := range imports {
		for pattern, name := range frameworkMap {
			if strings.Contains(strings.ToLower(imp), pattern) {
				frameworks = appendUnique(frameworks, name)
			}
		}
	}

	return frameworks
}

// detectTypeScriptRiskTags detects risk areas in TypeScript/JS code
func detectTypeScriptRiskTags(text string, imports []string) []string {
	var tags []string

	riskPatterns := map[string]string{
		"crypto":                  "crypto",
		"bcrypt":                  "auth",
		"jwt":                     "auth",
		"passport":                "auth",
		"sql":                     "database",
		"prisma":                  "database",
		"typeorm":                 "database",
		"mongoose":                "database",
		"eval(":                   "code-execution",
		"Function(":               "code-execution",
		"innerHTML":               "xss-risk",
		"dangerouslySetInnerHTML": "xss-risk",
		"child_process":           "subprocess",
		"exec(":                   "subprocess",
		"spawn(":                  "subprocess",
	}

	for _, imp := range imports {
		for pattern, tag := range riskPatterns {
			if strings.Contains(strings.ToLower(imp), pattern) {
				tags = appendUnique(tags, tag)
			}
		}
	}

	for pattern, tag := range riskPatterns {
		if strings.Contains(text, pattern) {
			tags = appendUnique(tags, tag)
		}
	}

	return tags
}

// extractFlutterSymbols extracts symbols from Dart/Flutter code
func extractFlutterSymbols(path string, content []byte) (*SymbolInfo, error) {
	info := &SymbolInfo{}
	text := string(content)

	// Imports
	importRe := regexp.MustCompile(`(?m)^import\s+['"]([^'"]+)['"]`)
	for _, match := range importRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Imports = append(info.Imports, match[1])
		}
	}

	// Classes (including widgets)
	classRe := regexp.MustCompile(`(?m)^(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?`)
	for _, match := range classRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			ti := TypeInfo{
				Name:       match[1],
				Kind:       "class",
				IsExported: !strings.HasPrefix(match[1], "_"),
			}
			if len(match) > 2 && match[2] != "" {
				// Detect widget types
				switch match[2] {
				case "StatelessWidget", "StatefulWidget", "Widget":
					ti.Kind = "widget"
				case "State":
					ti.Kind = "state"
				}
			}
			info.Types = append(info.Types, ti)
		}
	}

	// Mixins
	mixinRe := regexp.MustCompile(`(?m)^mixin\s+(\w+)`)
	for _, match := range mixinRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			info.Types = append(info.Types, TypeInfo{
				Name:       match[1],
				Kind:       "mixin",
				IsExported: !strings.HasPrefix(match[1], "_"),
			})
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:\w+\s+)?(\w+)\s*\([^)]*\)\s*(?:async\s*)?\{`)
	for _, match := range funcRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			name := match[1]
			// Skip common method names that are part of classes
			if name != "build" && name != "initState" && name != "dispose" {
				info.Functions = append(info.Functions, FunctionInfo{
					Name:       name,
					IsExported: !strings.HasPrefix(name, "_"),
				})
			}
		}
	}

	// Detect frameworks
	info.Frameworks = detectFlutterFrameworks(info.Imports)
	info.RiskTags = detectFlutterRiskTags(text, info.Imports)

	return info, nil
}

// detectFlutterFrameworks detects common Flutter packages
func detectFlutterFrameworks(imports []string) []string {
	var frameworks []string
	frameworkMap := map[string]string{
		"flutter":  "flutter",
		"provider": "provider",
		"bloc":     "bloc",
		"riverpod": "riverpod",
		"getx":     "getx",
		"dio":      "dio",
		"http":     "http",
		"sqflite":  "sqflite",
		"hive":     "hive",
		"firebase": "firebase",
		"test":     "test",
	}

	for _, imp := range imports {
		for pattern, name := range frameworkMap {
			if strings.Contains(strings.ToLower(imp), pattern) {
				frameworks = appendUnique(frameworks, name)
			}
		}
	}

	return frameworks
}

// detectFlutterRiskTags detects risk areas in Flutter/Dart code
func detectFlutterRiskTags(text string, imports []string) []string {
	var tags []string

	riskPatterns := map[string]string{
		"crypto":        "crypto",
		"encrypt":       "crypto",
		"firebase_auth": "auth",
		"sqflite":       "database",
		"hive":          "database",
		"http":          "network",
		"dio":           "network",
		"process":       "subprocess",
	}

	for _, imp := range imports {
		for pattern, tag := range riskPatterns {
			if strings.Contains(strings.ToLower(imp), pattern) {
				tags = appendUnique(tags, tag)
			}
		}
	}

	return tags
}

// Helper functions

func isUpperCase(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
