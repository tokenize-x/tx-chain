package deterministicmaplint

import (
	"go/ast"
	"go/types"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("deterministicmaplint", New)
}

// LinterSettings is the settings for the linter to enforce use of DeterministicMap instead of built-in map types.
type LinterSettings struct{}

// PluginDeterministicMapLint is the linter plugin.
type PluginDeterministicMapLint struct {
	settings LinterSettings
}

// New returns a new linter plugin.
func New(settings any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[LinterSettings](settings)
	if err != nil {
		return nil, err
	}

	return &PluginDeterministicMapLint{settings: s}, nil
}

// BuildAnalyzers returns the analyzers for the linter.
func (f *PluginDeterministicMapLint) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "deterministicmaplint",
			Doc:  "Disallow built-in map types; enforce DeterministicMap",
			Run:  f.run,
		},
	}, nil
}

// GetLoadMode returns the load mode for the linter.
func (f *PluginDeterministicMapLint) GetLoadMode() string {
	// NOTE: the mode can be `register.LoadModeSyntax` or `register.LoadModeTypesInfo`.
	// - `register.LoadModeSyntax`: if the linter doesn't use types information.
	// - `register.LoadModeTypesInfo`: if the linter uses types information.

	return register.LoadModeSyntax
}

func (f *PluginDeterministicMapLint) run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				if node.Type != nil {
					checkType(pass, node.Type, pass.TypesInfo.TypeOf(node.Type))
				}

			case *ast.Field:
				checkType(pass, node.Type, pass.TypesInfo.TypeOf(node.Type))

			case *ast.TypeSpec:
				checkType(pass, node.Type, pass.TypesInfo.TypeOf(node.Type))

			case *ast.CompositeLit:
				checkType(pass, node, pass.TypesInfo.TypeOf(node))

			case *ast.FuncDecl:
				checkFieldList(pass, node.Type.Params)
				checkFieldList(pass, node.Type.Results)
			}

			return true
		})
	}

	return nil, nil //nolint:nilnil
}

func checkFieldList(pass *analysis.Pass, fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		checkType(pass, f.Type, pass.TypesInfo.TypeOf(f.Type))
	}
}

func checkType(pass *analysis.Pass, node ast.Node, t types.Type) {
	if t == nil {
		return
	}
	if isForbiddenMapType(t) {
		pass.Reportf(
			node.Pos(),
			"use of built-in map is forbidden. use DeterministicMap instead",
		)
	}
}

func isForbiddenMapType(t types.Type) bool {
	for {
		switch tt := t.(type) {
		case *types.Named:
			// Explicit allow: DeterministicMap
			if obj := tt.Obj(); obj != nil && obj.Name() == "DeterministicMap" {
				return false
			}
			t = tt.Underlying()

		case *types.Map:
			return true

		default:
			return false
		}
	}
}
