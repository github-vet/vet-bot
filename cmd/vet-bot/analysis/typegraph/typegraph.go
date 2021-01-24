package typegraph

import (
	"go/ast"
	"go/types"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides a callgraph structure built to describe function signatures that pass interesting
// parameters. Expects type-checking information corresponding to the Uses, Defs, and Selections maps on the
// types.Info struct.
var Analyzer = &analysis.Analyzer{
	Name:             "typegraph",
	Doc:              "computes a callgraph for the provided files, using type information available.",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	// Declarations is a map from functions to the declarations found in the AST. All values in this map
	// are either of type *ast.FuncDecl or *ast.Field (in case of an interface type).
	Declarations map[*types.Func]ast.Node
	// ExternalCalls is a set of source positions for CallExprs which do not call into declared functions.
	// Calls in this map are into functions not declared into the current source, which are also verified
	// not to call builtin functions or casts to known types.
	ExternalCalls []*ast.CallExpr
	// CallGraph is the callgraph produced
	CallGraph CallGraph
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	callFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.InterfaceType)(nil),
		(*ast.CallExpr)(nil),
	}

	result := Result{
		Declarations: make(map[*types.Func]ast.Node),
		CallGraph:    NewCallGraph(),
	}
	inspect.WithStack(callFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			stats.AddCount(stats.StatFuncDecl, 1)
			// add declarations to the list of signatures which have declarations.
			fun := FuncDeclType(pass.TypesInfo, typed)
			if fun == nil {
				return true
			}
			result.Declarations[fun] = typed

		case *ast.InterfaceType:
			for _, field := range typed.Methods.List {
				fun := interfaceFieldType(pass.TypesInfo, field)
				result.Declarations[fun] = field
			}

		case *ast.CallExpr:
			stats.AddCount(stats.StatFuncCalls, 1)
			// retrieve type of Expression
			call, external := CallExprType(pass.TypesInfo, typed)
			if call == nil {
				if external {
					stats.AddCount(stats.StatExternalCalls, 1)
					result.ExternalCalls = append(result.ExternalCalls, typed)
				}
				return true
			}

			if !InterestingFunc(call) {
				return true
			}

			// retrieve the type of the enclosing function declaration
			caller := outermostFunc(pass.TypesInfo, stack)
			if caller == nil {
				return false
			}

			if !InterestingFunc(caller) {
				return true
			}

			// add calls from interesting callers to the call graph.
			callID := result.CallGraph.AddFunc(call)
			callerID := result.CallGraph.AddFunc(caller)
			result.CallGraph.AddCall(callerID, callID)
		}
		return true
	})
	return &result, nil
}

// CallExprType retrieves the type underlying the provided CallExpr, handling qualified
// identifiers and SelectorExpressions. Returns the func type, if present, and a boolean
// which is true if the call is into external code (i.e. not a cast to a known type or
// a built-in function).
func CallExprType(info *types.Info, call *ast.CallExpr) (*types.Func, bool) {
	switch typed := call.Fun.(type) {
	case *ast.SelectorExpr:
		if sel, ok := info.Uses[typed.Sel]; ok {
			if fun, ok := sel.(*types.Func); ok {
				return fun, false
			}
			return nil, false
		} else {
			if s, ok := info.Selections[typed]; ok {
				return s.Obj().(*types.Func), false
			}
			return nil, true
		}
	case *ast.Ident:
		switch fun := info.Uses[typed].(type) {
		case *types.Func:
			return fun, false
		case *types.Builtin:
			return nil, false // built-in functions are not external
		case *types.TypeName:
			return nil, false // casts to known types are not external calls
		}
		return nil, true // CallExpr casts to some external type
	default:
		return nil, false
	}

}

// FuncDeclType retrieves the type underlying the provided FuncDecl.
func FuncDeclType(info *types.Info, fdec *ast.FuncDecl) *types.Func {
	if def, ok := info.Defs[fdec.Name]; ok {
		if fun, ok := def.(*types.Func); ok {
			return fun
		}
	}
	return nil
}

// interfaceFieldType retrieves type info underlying the provided interface field.
func interfaceFieldType(info *types.Info, field *ast.Field) *types.Func {
	if len(field.Names) > 0 {
		if def, ok := info.Defs[field.Names[0]]; ok {
			if fun, ok := def.(*types.Func); ok {
				return fun
			}
		}
	}
	return nil
}

func InterestingSignature(sig *types.Signature) bool {
	// check for pointer receiver
	v := sig.Recv()
	if v != nil {
		if _, ok := v.Type().(*types.Pointer); ok {
			return true
		}
	}
	// check for pointer arguments or empty interfaces
	for i := 0; i < sig.Params().Len(); i++ {
		if InterestingParameter(sig.Params().At(i).Type()) {
			return true
		}
	}
	// handle variadic arguments
	if sig.Variadic() {
		slice, ok := sig.Params().At(sig.Params().Len() - 1).Type().(*types.Slice)
		if !ok {
			return false // type-checker did something wrong
		}
		if InterestingParameter(slice.Elem()) {
			return true
		}
	}
	return false
}

func InterestingParameter(param types.Type) bool {
	switch typed := param.(type) {
	case *types.Pointer:
		return true
	case *types.Interface:
		if typed.Empty() {
			return true
		}
	}
	return false
}

// InterestingFunc returns true if the provided signature has a pointer receiver,
// or an argument which is a pointer or an empty interface. Variadic arguments are supported.
func InterestingFunc(fun *types.Func) bool {
	if fun == nil {
		return true // missing type declarations are always interesting
	}
	sig, ok := fun.Type().(*types.Signature)
	if !ok {
		return false
	}
	return InterestingSignature(sig)
}

// outermostFunc returns the types.Object associated with the outermost FuncDecl in the provided stack.
func outermostFunc(info *types.Info, stack []ast.Node) *types.Func {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			if def, ok := info.Defs[decl.Name]; ok {
				return def.(*types.Func)
			}
			return nil
		}
	}
	return nil
}
