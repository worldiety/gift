// Command apigen writes the API tables of the documentation site.
//
// The component pages of docs/gift.dev list the constructors and methods of
// each view type. Nago maintains such tables by hand and reviews them every
// quarter; here they are read from the source with go/doc, so a renamed or new
// method reaches the site with the next run instead of the next review.
//
//	go run ./docs/apigen     # rewrites docs/gift.dev/data/api.json
//
// TestAPIDataIsCurrent fails when the committed file is out of date.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// packages are the import paths, relative to the module, that the site
// documents.
var packages = []string{
	".",
	"ui",
	"ui/filepicker",
	"geom",
	"asset",
	"backend/ebiten",
	"gifttest",
}

const module = "github.com/worldiety/gift"

var out = flag.String("o", "docs/gift.dev/data/api.json", "output file")

func main() {
	flag.Parse()
	b, err := generate(os.DirFS("."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "apigen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "apigen:", err)
		os.Exit(1)
	}
}

// API is the content of api.json: packages by their short name, as they are
// written in code, e.g. "ui" or "gift".
type API map[string]Package

type Package struct {
	ImportPath string          `json:"importPath"`
	Synopsis   string          `json:"synopsis"`
	Types      map[string]Type `json:"types"`
	Funcs      []Func          `json:"funcs"`
}

type Type struct {
	Synopsis     string `json:"synopsis"`
	Decl         string `json:"decl"`
	Constructors []Func `json:"constructors"`
	Methods      []Func `json:"methods"`
}

type Func struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Synopsis  string `json:"synopsis"`
}

func generate(fsys fs.FS) ([]byte, error) {
	api := API{}
	for _, dir := range packages {
		p, err := load(fsys, dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		short := p.Name
		pkg := Package{
			ImportPath: path.Join(module, dir),
			Synopsis:   p.Synopsis(p.Doc),
			Types:      map[string]Type{},
			Funcs:      funcs(p, p.Funcs),
		}
		fset := token.NewFileSet()
		for _, t := range p.Types {
			if !ast.IsExported(t.Name) {
				continue
			}
			pkg.Types[t.Name] = Type{
				Synopsis:     p.Synopsis(t.Doc),
				Decl:         typeDecl(fset, t),
				Constructors: funcs(p, t.Funcs),
				Methods:      funcs(p, t.Methods),
			}
		}
		api[short] = pkg
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(api); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// load parses the non test files of one package directory, without build
// tags, so that the giftauto and giftdebug variants do not leak in.
func load(fsys fs.FS, dir string) (*doc.Package, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := fs.ReadFile(fsys, path.Join(dir, n))
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, n, src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if constrained(f) {
			continue
		}
		files = append(files, f)
	}
	return doc.NewFromFiles(fset, files, path.Join(module, dir))
}

// constrained reports whether the file has a build constraint other than one
// that is satisfied by a default build, which here means: any constraint that
// does not start with a negation.
func constrained(f *ast.File) bool {
	for _, g := range f.Comments {
		if g.Pos() > f.Package {
			break
		}
		for _, c := range g.List {
			if expr, ok := strings.CutPrefix(c.Text, "//go:build "); ok {
				return !strings.HasPrefix(strings.TrimSpace(expr), "!")
			}
		}
	}
	return false
}

// hidden are the methods every view has for the runtime and that an
// application never calls.
var hidden = map[string]bool{"Build": true, "ViewType": true}

func funcs(p *doc.Package, fs []*doc.Func) []Func {
	fset := token.NewFileSet()
	out := make([]Func, 0, len(fs))
	for _, f := range fs {
		if !ast.IsExported(f.Name) || (f.Recv != "" && hidden[f.Name]) {
			continue
		}
		out = append(out, Func{
			Name:      f.Name,
			Signature: signature(fset, f.Decl),
			Synopsis:  p.Synopsis(f.Doc),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// signature prints a function declaration without its body and, for a
// method, without its receiver: "Title(text string) ButtonView".
func signature(fset *token.FileSet, d *ast.FuncDecl) string {
	decl := *d
	decl.Body = nil
	decl.Doc = nil
	decl.Recv = nil
	var buf bytes.Buffer
	_ = pretty.Fprint(&buf, fset, &decl)
	return strings.TrimPrefix(buf.String(), "func ")
}

// typeDecl prints the declaration of a type without its documentation, the
// way go doc shows it; go/doc has already dropped the unexported fields.
func typeDecl(fset *token.FileSet, t *doc.Type) string {
	decl := *t.Decl
	decl.Doc = nil
	specs := make([]ast.Spec, 0, len(decl.Specs))
	for _, s := range decl.Specs {
		ts, ok := s.(*ast.TypeSpec)
		if !ok || ts.Name.Name != t.Name {
			continue
		}
		c := *ts
		c.Doc, c.Comment = nil, nil
		if st, ok := c.Type.(*ast.StructType); ok {
			c.Type = withoutComments(st)
		}
		specs = append(specs, &c)
	}
	decl.Specs = specs
	decl.Lparen, decl.Rparen = 0, 0
	var buf bytes.Buffer
	_ = pretty.Fprint(&buf, fset, &decl)
	s := buf.String()
	if hiddenFields(t) {
		s = strings.TrimSuffix(s, "}")
		if body, ok := strings.CutSuffix(s, "struct{"); ok {
			s = body + "struct {\n"
		}
		s += "\t// Has unexported fields.\n}"
	}
	return s
}

// pretty aligns with spaces, the way gofmt and go doc do.
var pretty = printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}

// hiddenFields reports whether the struct type t has unexported fields;
// go/doc has already removed them and marked the field list incomplete.
func hiddenFields(t *doc.Type) bool {
	for _, s := range t.Decl.Specs {
		if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == t.Name {
			st, ok := ts.Type.(*ast.StructType)
			return ok && st.Incomplete
		}
	}
	return false
}

// withoutComments copies a struct type without its field comments and without
// the incomplete mark, which the printer would otherwise render on the line of
// the closing brace; typeDecl writes that note itself.
func withoutComments(st *ast.StructType) *ast.StructType {
	fields := &ast.FieldList{Opening: st.Fields.Opening, Closing: st.Fields.Closing}
	for _, f := range st.Fields.List {
		c := *f
		c.Doc, c.Comment = nil, nil
		fields.List = append(fields.List, &c)
	}
	return &ast.StructType{Struct: st.Struct, Fields: fields}
}
