package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/PFC-Umc-Organization/PFC.Backend/internal/"

// allowed lists, for each internal package, the internal packages it may
// import. A new dependency between modules has to be added here on purpose.
var allowed = map[string][]string{
	"platform/actor":   nil,
	"platform/apperr":  nil,
	"platform/audit":   nil,
	"platform/config":  nil,
	"platform/dynamo":  nil,
	"platform/httpapi": {"platform/actor", "platform/apperr", "platform/audit"},

	"identity":   platform,
	"academic":   platform,
	"groups":     platform,
	"deliveries": append([]string{"groups"}, platform...),
	"references": append([]string{"groups"}, platform...),

	"app":        {"*"},
	"integracao": {"app", "platform/dynamo", "platform/httpapi"},
}

var platform = []string{"platform/actor", "platform/apperr", "platform/audit", "platform/dynamo", "platform/httpapi"}

func TestModuleBoundaries(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkg := filepath.ToSlash(rel)
		rules, known := allowed[pkg]
		if !known {
			t.Errorf("%s has no entry in the boundary rules", pkg)
			return nil
		}
		seen[pkg] = true
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			target, _ := strconv.Unquote(imp.Path.Value)
			dep, internal := strings.CutPrefix(target, module)
			if !internal || slices.Contains(rules, "*") || slices.Contains(rules, dep) {
				continue
			}
			t.Errorf("%s imports %s, which the boundary rules do not allow", filepath.Base(path), dep)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg := range allowed {
		if !seen[pkg] {
			t.Errorf("boundary rule for %s, which no longer exists", pkg)
		}
	}
}
