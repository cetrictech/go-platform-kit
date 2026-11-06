// Command api-surface checks reviewed optional-driver types in public Go signatures.
// It parses source only: it does not load dependencies, execute initializers, or inspect bodies.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type policy struct {
	SchemaVersion        int                 `json:"schema_version"`
	DriverImportPrefixes []string            `json:"driver_import_prefixes"`
	Approved             map[string][]string `json:"approved"`
}
type declaration struct {
	spec *ast.TypeSpec
	file *sourceFile
}
type methodDeclaration struct {
	signature *ast.FuncType
	file      *sourceFile
}
type sourceFile struct {
	ast     *ast.File
	imports map[string]string
	dots    []string
	pkg     *sourcePackage
}
type sourcePackage struct {
	path    string
	files   []*sourceFile
	types   map[string][]declaration
	methods map[string][]methodDeclaration
}
type scanner struct {
	root     string
	prefixes []string
	packages map[string]*sourcePackage
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "API surface gate:", err)
		os.Exit(1)
	}
}
func execute(args []string) error {
	flags := flag.NewFlagSet("api-surface", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	allow := flags.String("allowlist", "release/driver-exposure.json", "reviewed policy, relative to root")
	candidate := flags.String("write-candidate", "", "write a NEW review candidate file; never overwrite the policy")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	policyPath := *allow
	if !filepath.IsAbs(policyPath) {
		policyPath = filepath.Join(absolute, policyPath)
	}
	var approved policy
	bytes, err := os.ReadFile(policyPath)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(bytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&approved); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("policy must contain one JSON object")
	}
	if err := validatePolicy(approved); err != nil {
		return err
	}
	s := scanner{root: absolute, prefixes: approved.DriverImportPrefixes, packages: map[string]*sourcePackage{}}
	if err := s.load(); err != nil {
		return err
	}
	actual := s.inventory()
	encoded, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if *candidate != "" {
		path := *candidate
		if !filepath.IsAbs(path) {
			path = filepath.Join(absolute, path)
		}
		if samePath(path, policyPath) {
			return errors.New("--write-candidate cannot overwrite the approved policy")
		}
		next := policy{SchemaVersion: 1, DriverImportPrefixes: approved.DriverImportPrefixes, Approved: actual}
		content, err := json.MarshalIndent(next, "", "  ")
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := out.Write(append(content, '\n'))
		closeErr := out.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Review candidate written:", path)
		return nil
	}
	var changes []string
	for symbol, imports := range actual {
		expected, ok := approved.Approved[symbol]
		if !ok {
			changes = append(changes, symbol+": unreviewed exposure to "+strings.Join(imports, ", "))
			continue
		}
		if !equalStrings(imports, expected) {
			changes = append(changes, symbol+": driver imports changed; expected "+strings.Join(expected, ", ")+", found "+strings.Join(imports, ", "))
		}
	}
	for symbol := range approved.Approved {
		if _, ok := actual[symbol]; !ok {
			changes = append(changes, symbol+": stale approval; review removal from policy")
		}
	}
	if len(changes) > 0 {
		sort.Strings(changes)
		return errors.New(strings.Join(changes, "\n"))
	}
	fmt.Fprintln(os.Stderr, "Public driver exposure matches the reviewed policy.")
	return nil
}
func validatePolicy(p policy) error {
	if p.SchemaVersion != 1 || len(p.DriverImportPrefixes) == 0 || p.Approved == nil {
		return errors.New("invalid driver-exposure policy schema")
	}
	seen := map[string]bool{}
	for _, prefix := range p.DriverImportPrefixes {
		if prefix == "" || strings.TrimSpace(prefix) != prefix || seen[prefix] {
			return errors.New("invalid or duplicate driver import prefix")
		}
		seen[prefix] = true
	}
	for name, imports := range p.Approved {
		if name == "" || len(imports) == 0 || !sort.StringsAreSorted(imports) {
			return errors.New("approvals must have nonempty symbol keys and sorted driver imports")
		}
		previous := ""
		for _, path := range imports {
			if path == previous || !matches(path, p.DriverImportPrefixes) {
				return fmt.Errorf("invalid approved driver import for %s", name)
			}
			previous = path
		}
	}
	return nil
}
func samePath(a, b string) bool {
	resolve := func(path string) string {
		if actual, err := filepath.EvalSymlinks(path); err == nil {
			return actual
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err == nil {
			return filepath.Join(parent, filepath.Base(path))
		}
		return filepath.Clean(path)
	}
	return resolve(a) == resolve(b)
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func matches(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
func skipped(name string) bool {
	return strings.HasPrefix(name, ".") || name == "internal" || name == "integration" || name == "vendor" || name == "testdata"
}
func (s *scanner) load() error {
	return filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != s.root && skipped(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		packagePath, err := s.packagePath(filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := s.packages[packagePath]
		if pkg == nil {
			pkg = &sourcePackage{path: packagePath, types: map[string][]declaration{}, methods: map[string][]methodDeclaration{}}
			s.packages[packagePath] = pkg
		}
		file := &sourceFile{ast: tree, pkg: pkg, imports: map[string]string{}}
		for _, imp := range tree.Imports {
			imported, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			alias := defaultAlias(imported)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				if matches(imported, s.prefixes) {
					file.dots = append(file.dots, imported)
				}
			} else if alias != "_" {
				file.imports[alias] = imported
			}
		}
		pkg.files = append(pkg.files, file)
		for _, decl := range tree.Decls {
			if method, ok := decl.(*ast.FuncDecl); ok && method.Recv != nil && len(method.Recv.List) == 1 && ast.IsExported(method.Name.Name) {
				receiver := baseName(method.Recv.List[0].Type)
				pkg.methods[receiver] = append(pkg.methods[receiver], methodDeclaration{method.Type, file})
			}
			group, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range group.Specs {
				if typed, ok := spec.(*ast.TypeSpec); ok {
					pkg.types[typed.Name.Name] = append(pkg.types[typed.Name.Name], declaration{typed, file})
				}
			}
		}
		return nil
	})
}

// Go package names usually match import basenames; these reviewed driver roots do not.
func defaultAlias(path string) string {
	name := filepath.Base(path)
	if isVersionSegment(name) {
		name = filepath.Base(filepath.Dir(path))
	}
	if name == "go-redis" {
		return "redis"
	}
	if name == "go-spiffe" {
		return "spiffe"
	}
	return name
}
func isVersionSegment(name string) bool {
	if len(name) < 2 || name[0] != 'v' {
		return false
	}
	digit := false
	for _, r := range name[1:] {
		if r >= '0' && r <= '9' {
			digit = true
			continue
		}
		if r == '.' && digit {
			digit = false
			continue
		}
		return false
	}
	return digit
}
func (s *scanner) packagePath(dir string) (string, error) {
	current := dir
	for {
		data, err := os.ReadFile(filepath.Join(current, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && parts[0] == "module" {
					module := parts[1]
					if strings.HasPrefix(module, "\"") {
						module, err = strconv.Unquote(module)
						if err != nil {
							return "", err
						}
					}
					relative, err := filepath.Rel(current, dir)
					if err != nil {
						return "", err
					}
					if relative == "." {
						return module, nil
					}
					return module + "/" + filepath.ToSlash(relative), nil
				}
			}
			return "", fmt.Errorf("module directive missing: %s", current)
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if current == s.root {
			return "", fmt.Errorf("source outside a Go module: %s", dir)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("module root not found")
		}
		current = parent
	}
}
func (s *scanner) inventory() map[string][]string {
	output := map[string][]string{}
	add := func(symbol string, file *sourceFile, nodes ...ast.Node) {
		found := map[string]bool{}
		seen := map[string]bool{}
		for _, node := range nodes {
			s.inspect(node, file, found, seen)
		}
		if len(found) == 0 {
			return
		}
		key := file.pkg.path + "." + symbol
		set := map[string]bool{}
		for _, item := range output[key] {
			set[item] = true
		}
		for item := range found {
			set[item] = true
		}
		var imports []string
		for item := range set {
			imports = append(imports, item)
		}
		sort.Strings(imports)
		output[key] = imports
	}
	for _, pkg := range s.packages {
		for _, file := range pkg.files {
			for _, decl := range file.ast.Decls {
				switch item := decl.(type) {
				case *ast.FuncDecl:
					if !ast.IsExported(item.Name.Name) {
						continue
					}
					name := item.Name.Name
					if item.Recv != nil {
						if len(item.Recv.List) != 1 {
							continue
						}
						receiver := baseName(item.Recv.List[0].Type)
						if !ast.IsExported(receiver) {
							continue
						}
						name = receiver + "." + name
					}
					add(name, file, item.Type)
				case *ast.GenDecl:
					for _, spec := range item.Specs {
						switch typed := spec.(type) {
						case *ast.TypeSpec:
							if !ast.IsExported(typed.Name.Name) {
								continue
							}
							add(typed.Name.Name, file, typed.Type, typed.TypeParams)
							if body, ok := typed.Type.(*ast.StructType); ok {
								for _, field := range body.Fields.List {
									for _, name := range field.Names {
										if ast.IsExported(name.Name) {
											add(typed.Name.Name+"."+name.Name, file, field.Type)
										}
									}
									if len(field.Names) == 0 {
										if name := baseName(field.Type); ast.IsExported(name) {
											add(typed.Name.Name+"."+name, file, field.Type)
										}
									}
								}
							}
							if body, ok := typed.Type.(*ast.InterfaceType); ok {
								for _, field := range body.Methods.List {
									for _, name := range field.Names {
										if ast.IsExported(name.Name) {
											add(typed.Name.Name+"."+name.Name, file, field.Type)
										}
									}
								}
							}
						case *ast.ValueSpec:
							if typed.Type == nil {
								continue
							}
							for _, name := range typed.Names {
								if ast.IsExported(name.Name) {
									add(name.Name, file, typed.Type)
								}
							}
						}
					}
				}
			}
		}
	}
	return output
}
func baseName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.StarExpr:
		return baseName(x.X)
	case *ast.IndexExpr:
		return baseName(x.X)
	case *ast.IndexListExpr:
		return baseName(x.X)
	case *ast.ParenExpr:
		return baseName(x.X)
	}
	return ""
}
func (s *scanner) named(name string, pkg *sourcePackage, found, seen map[string]bool) {
	key := pkg.path + "." + name
	if seen[key] {
		return
	}
	seen[key] = true
	for _, decl := range pkg.types[name] {
		s.inspect(decl.spec.Type, decl.file, found, seen)
		s.inspect(decl.spec.TypeParams, decl.file, found, seen)
	}
	// An unexported type returned by a public function still exposes its exported methods.
	if !ast.IsExported(name) {
		for _, method := range pkg.methods[name] {
			s.inspect(method.signature, method.file, found, seen)
		}
	}
}
func (s *scanner) inspect(node ast.Node, file *sourceFile, found, seen map[string]bool) {
	switch x := node.(type) {
	case nil:
		return
	case *ast.Ident:
		if len(file.pkg.types[x.Name]) > 0 {
			s.named(x.Name, file.pkg, found, seen)
		} else if ast.IsExported(x.Name) {
			for _, path := range file.dots {
				found[path] = true
			}
		}
	case *ast.SelectorExpr:
		if alias, ok := x.X.(*ast.Ident); ok {
			path := file.imports[alias.Name]
			if matches(path, s.prefixes) {
				found[path] = true
			}
			if pkg := s.packages[path]; pkg != nil {
				s.named(x.Sel.Name, pkg, found, seen)
			}
		}
	case *ast.StarExpr:
		s.inspect(x.X, file, found, seen)
	case *ast.ArrayType:
		s.inspect(x.Len, file, found, seen)
		s.inspect(x.Elt, file, found, seen)
	case *ast.MapType:
		s.inspect(x.Key, file, found, seen)
		s.inspect(x.Value, file, found, seen)
	case *ast.ChanType:
		s.inspect(x.Value, file, found, seen)
	case *ast.Ellipsis:
		s.inspect(x.Elt, file, found, seen)
	case *ast.ParenExpr:
		s.inspect(x.X, file, found, seen)
	case *ast.IndexExpr:
		s.inspect(x.X, file, found, seen)
		s.inspect(x.Index, file, found, seen)
	case *ast.IndexListExpr:
		s.inspect(x.X, file, found, seen)
		for _, index := range x.Indices {
			s.inspect(index, file, found, seen)
		}
	case *ast.UnaryExpr:
		s.inspect(x.X, file, found, seen)
	case *ast.BinaryExpr:
		s.inspect(x.X, file, found, seen)
		s.inspect(x.Y, file, found, seen)
	case *ast.FuncType:
		s.inspect(x.TypeParams, file, found, seen)
		s.inspect(x.Params, file, found, seen)
		s.inspect(x.Results, file, found, seen)
	case *ast.FieldList:
		if x != nil {
			for _, field := range x.List {
				s.inspect(field.Type, file, found, seen)
			}
		}
	case *ast.StructType:
		for _, field := range x.Fields.List {
			visible := len(field.Names) == 0
			for _, name := range field.Names {
				visible = visible || ast.IsExported(name.Name)
			}
			if visible {
				s.inspect(field.Type, file, found, seen)
			}
		}
	case *ast.InterfaceType:
		for _, method := range x.Methods.List {
			visible := len(method.Names) == 0
			for _, name := range method.Names {
				visible = visible || ast.IsExported(name.Name)
			}
			if visible {
				s.inspect(method.Type, file, found, seen)
			}
		}
	}
}
