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
			if rs, isRange := n.(*ast.RangeStmt); isRange {
				t := pass.TypesInfo.TypeOf(rs.X)
				if t == nil {
					return true
				}

				if isForbiddenMapType(t) {
					pass.Reportf(
						rs.Pos(),
						"ranging over map is forbidden (iteration order is nondeterministic); use DeterministicMap instead",
					)
				}
			}

			return true
		})
	}

	return nil, nil //nolint:nilnil
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
