package engine_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	talaInternalImport  = "github.com/d2lang/d2/d2layouts/d2talalayout/internal/"
	talaInternalRoot    = "github.com/d2lang/d2/d2layouts/d2talalayout/internal"
	engineImport        = talaInternalImport + "engine"
	graphBoundsImport   = talaInternalImport + "graphbounds"
	graphJSONImport     = talaInternalImport + "graphjson"
	placementImport     = talaInternalImport + "placement"
	placementCostImport = talaInternalImport + "placementcost"
)

var hostModelImports = []string{
	"github.com/d2lang/d2/d2renderers/d2fonts",
	"github.com/d2lang/d2/d2graph",
	"github.com/d2lang/d2/d2target",
}

var algorithmPackageNames = []string{
	"grouping",
	"proximity",
	"trees",
	"hierarchy",
	"packing",
	"placement",
	"labeling",
	"loops",
	"routing",
	"quality",
}

var allowedAlgorithmImports = map[string][]string{
	"placement": {"grouping", "labeling", "loops", "packing", "proximity", "trees"},
	"routing":   {"hierarchy", "labeling", "loops", "quality"},
	"quality":   {"labeling"},
}

type packageImportRule struct {
	packageImport    string
	forbiddenImports []string
	forbiddenPrefix  string
}

// TestInternalPackageDependencies keeps the mutable graph representation and
// layout algorithms below the engine orchestrator. Algorithm-to-algorithm
// imports are limited to the concrete collaboration edges used by the layout
// pipeline; serialized graph records do not leak into those algorithms.
func TestInternalPackageDependencies(t *testing.T) {
	algorithmImports := make([]string, 0, len(algorithmPackageNames))
	algorithmImportByName := make(map[string]string, len(algorithmPackageNames))
	for _, name := range algorithmPackageNames {
		algorithmImport := talaInternalImport + name
		algorithmImports = append(algorithmImports, algorithmImport)
		algorithmImportByName[name] = algorithmImport
	}

	layoutGraphForbidden := []string{
		engineImport,
		graphBoundsImport,
		graphJSONImport,
		talaInternalImport + "layouttxn",
	}
	layoutGraphForbidden = append(layoutGraphForbidden, hostModelImports...)
	layoutGraphForbidden = append(layoutGraphForbidden, algorithmImports...)
	graphJSONForbidden := append([]string{engineImport, graphBoundsImport}, hostModelImports...)
	graphJSONForbidden = append(graphJSONForbidden, algorithmImports...)
	graphBoundsForbidden := append([]string{engineImport, graphJSONImport}, hostModelImports...)
	graphBoundsForbidden = append(graphBoundsForbidden, algorithmImports...)

	rules := []packageImportRule{
		{
			packageImport:    talaInternalImport + "layoutgraph",
			forbiddenImports: layoutGraphForbidden,
		},
		{
			// graphbounds is shared lower geometry over layoutgraph. It must not
			// depend back on packing, routing, or another layout algorithm.
			packageImport:    talaInternalImport + "graphbounds",
			forbiddenImports: graphBoundsForbidden,
		},
		{
			packageImport:    talaInternalImport + "nodeshape",
			forbiddenImports: hostModelImports,
			forbiddenPrefix:  talaInternalImport,
		},
		{
			packageImport:    talaInternalImport + "graphjson",
			forbiddenImports: graphJSONForbidden,
		},
		{
			// labelgeom is the one narrow renderer-compatibility leaf. It may
			// call d2target geometry but must never depend back on TALA.
			packageImport: talaInternalImport + "labelgeom",
			forbiddenImports: []string{
				"github.com/d2lang/d2/d2renderers/d2fonts",
				"github.com/d2lang/d2/d2graph",
			},
			forbiddenPrefix: talaInternalImport,
		},
		{
			packageImport:    engineImport,
			forbiddenImports: append([]string{graphJSONImport}, hostModelImports...),
		},
	}
	for _, name := range algorithmPackageNames {
		forbidden := append([]string{engineImport, graphJSONImport}, hostModelImports...)
		allowedSiblings := allowedAlgorithmImports[name]
		for _, siblingName := range algorithmPackageNames {
			if siblingName == name || slices.Contains(allowedSiblings, siblingName) {
				continue
			}
			forbidden = append(forbidden, algorithmImportByName[siblingName])
		}
		rules = append(rules, packageImportRule{
			packageImport:    algorithmImportByName[name],
			forbiddenImports: forbidden,
		})
	}
	// Catch utility packages not listed above: renderer/parser host models stay
	// outside internal TALA, with labelgeom's explicit d2target exception.
	rules = append(rules, packageImportRule{
		packageImport:    talaInternalRoot,
		forbiddenImports: hostModelImports,
	})

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate package-boundary test")
	}
	internalDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))

	fset := token.NewFileSet()
	err := filepath.WalkDir(internalDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != internalDir && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relativeDir, err := filepath.Rel(internalDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		packageImport := talaInternalImport + filepath.ToSlash(relativeDir)
		ruleIndex := slices.IndexFunc(rules, func(rule packageImportRule) bool {
			return isPackageOrDescendant(packageImport, rule.packageImport)
		})
		if ruleIndex < 0 {
			return nil
		}

		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rule := rules[ruleIndex]
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if isPackageOrDescendant(importPath, placementCostImport) &&
				!isPackageOrDescendant(packageImport, placementImport) {
				t.Errorf("%s imports placement scoring outside the placement package", filepath.ToSlash(path))
				continue
			}
			if isPackageOrDescendant(packageImport, placementCostImport) {
				standardLibrary := !strings.Contains(strings.Split(importPath, "/")[0], ".")
				allowedLeaf := importPath == "github.com/d2lang/d2/lib/geo" ||
					importPath == talaInternalImport+"invariant" ||
					importPath == talaInternalImport+"layoutgraph" ||
					importPath == talaInternalImport+"typedpool"
				if !standardLibrary && !allowedLeaf {
					t.Errorf("%s imports non-leaf placement-cost dependency %s", filepath.ToSlash(path), importPath)
				}
				continue
			}
			if slices.ContainsFunc(rule.forbiddenImports, func(forbidden string) bool {
				return isPackageOrDescendant(importPath, forbidden)
			}) ||
				(rule.forbiddenPrefix != "" && strings.HasPrefix(importPath, rule.forbiddenPrefix)) {
				t.Errorf("%s imports forbidden higher-level package %s", filepath.ToSlash(path), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAlgorithmPackagesDoNotAliasOwnershipAPIs rejects production type aliases,
// direct variable forwarders, and dot imports that obscure layoutgraph or
// sibling-algorithm ownership. Constants and functions are intentionally outside
// this syntactic rule and remain subject to normal code review.
func TestAlgorithmPackagesDoNotAliasOwnershipAPIs(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate package-boundary test")
	}
	internalDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))

	fset := token.NewFileSet()
	var violations []string
	err := filepath.WalkDir(internalDir, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filename != internalDir && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(filename) != ".go" || strings.HasSuffix(filename, "_test.go") {
			return nil
		}

		relativeDir, err := filepath.Rel(internalDir, filepath.Dir(filename))
		if err != nil {
			return err
		}
		packageImport := talaInternalImport + filepath.ToSlash(relativeDir)
		algorithmName, ok := algorithmPackageName(packageImport)
		if !ok {
			return nil
		}

		parsed, err := parser.ParseFile(fset, filename, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		relativeFile, err := filepath.Rel(internalDir, filename)
		if err != nil {
			return err
		}
		relativeFile = filepath.ToSlash(relativeFile)

		fileViolations, err := algorithmAliasViolations(fset, parsed, relativeFile, algorithmName)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(violations)
	for _, violation := range violations {
		t.Error(violation)
	}
}

func TestAlgorithmAliasPolicy(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "ordinary use and function wrapper",
			src: `package grouping
import layoutgraph "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
type state struct { node *layoutgraph.Node }
func newNode() *layoutgraph.Node { return layoutgraph.NewNode(1, 1, 1) }
const maxReferences = layoutgraph.MaxTopologyReferences
`,
		},
		{
			name: "type alias",
			src: `package grouping
import layoutgraph "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
type Node = layoutgraph.Node
`,
			want: []string{"aliases type Node"},
		},
		{
			name: "variable forwarder",
			src: `package grouping
import layoutgraph "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
var NewNode = layoutgraph.NewNode
`,
			want: []string{"forwards var NewNode"},
		},
		{
			name: "dot import",
			src: `package grouping
import . "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
`,
			want: []string{"dot-imports ownership package"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, "fixture.go", test.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			got, err := algorithmAliasViolations(fset, parsed, "fixture.go", "grouping")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("violations = %q, want %q", got, test.want)
			}
			for index, want := range test.want {
				if !strings.Contains(got[index], want) {
					t.Fatalf("violation %d = %q, want substring %q", index, got[index], want)
				}
			}
		})
	}
}

func algorithmAliasViolations(
	fset *token.FileSet,
	parsed *ast.File,
	relativeFile string,
	algorithmName string,
) ([]string, error) {
	var violations []string
	facadeImports := make(map[string]string)
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if !isAlgorithmFacadeTarget(importPath, algorithmName) {
			continue
		}

		importName := path.Base(importPath)
		if spec.Name != nil {
			importName = spec.Name.Name
		}
		switch importName {
		case ".":
			violations = append(violations, packageBoundaryViolation(
				fset, relativeFile, spec.Pos(), "dot-imports ownership package %s", importPath,
			))
		case "_":
			continue
		default:
			facadeImports[importName] = importPath
		}
	}

	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch general.Tok {
		case token.TYPE:
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if !typeSpec.Assign.IsValid() {
					continue
				}
				if importPath, ok := expressionFacadeImport(typeSpec.Type, facadeImports); ok {
					violations = append(violations, packageBoundaryViolation(
						fset, relativeFile, typeSpec.Pos(), "aliases type %s from ownership package %s", typeSpec.Name.Name, importPath,
					))
				}
			}
		case token.VAR:
			for _, spec := range general.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for _, value := range valueSpec.Values {
					if importPath, ok := forwardedFacadeImport(value, facadeImports); ok {
						names := make([]string, len(valueSpec.Names))
						for index, name := range valueSpec.Names {
							names[index] = name.Name
						}
						violations = append(violations, packageBoundaryViolation(
							fset, relativeFile, value.Pos(), "forwards var %s from ownership package %s", strings.Join(names, ", "), importPath,
						))
					}
				}
			}
		}
	}
	return violations, nil
}

func algorithmPackageName(packageImport string) (string, bool) {
	relative, found := strings.CutPrefix(packageImport, talaInternalImport)
	if !found {
		return "", false
	}
	name, _, _ := strings.Cut(relative, "/")
	return name, slices.Contains(algorithmPackageNames, name)
}

func isAlgorithmFacadeTarget(importPath, sourceAlgorithm string) bool {
	if isPackageOrDescendant(importPath, talaInternalImport+"layoutgraph") {
		return true
	}
	for _, algorithmName := range algorithmPackageNames {
		if algorithmName != sourceAlgorithm && isPackageOrDescendant(importPath, talaInternalImport+algorithmName) {
			return true
		}
	}
	return false
}

func expressionFacadeImport(expression ast.Expr, facadeImports map[string]string) (string, bool) {
	var found string
	ast.Inspect(expression, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if importPath, ok := facadeImports[identifier.Name]; ok {
			found = importPath
			return false
		}
		return true
	})
	return found, found != ""
}

func forwardedFacadeImport(expression ast.Expr, facadeImports map[string]string) (string, bool) {
	switch expression := expression.(type) {
	case *ast.Ident:
		return "", false
	case *ast.IndexExpr:
		return forwardedFacadeImport(expression.X, facadeImports)
	case *ast.IndexListExpr:
		return forwardedFacadeImport(expression.X, facadeImports)
	case *ast.ParenExpr:
		return forwardedFacadeImport(expression.X, facadeImports)
	case *ast.SelectorExpr:
		if identifier, ok := expression.X.(*ast.Ident); ok {
			importPath, ok := facadeImports[identifier.Name]
			return importPath, ok
		}
		return forwardedFacadeImport(expression.X, facadeImports)
	case *ast.StarExpr:
		return forwardedFacadeImport(expression.X, facadeImports)
	default:
		return "", false
	}
}

func packageBoundaryViolation(fset *token.FileSet, relativeFile string, position token.Pos, format string, args ...any) string {
	return relativeFile + ":" + strconv.Itoa(fset.Position(position).Line) + ": " + fmt.Sprintf(format, args...)
}

func isPackageOrDescendant(importPath, packageImport string) bool {
	return importPath == packageImport || strings.HasPrefix(importPath, packageImport+"/")
}
