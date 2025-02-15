package nogofunc

import (
	"go/ast"
	"go/token"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides a set of function signatures whose invocations start a goroutine.
// False-positives should be expected, as no type-checking information is used during the
// analysis, which relies only on approximate knowledge of the call-graph.
var Analyzer = &analysis.Analyzer{
	Name:             "nogofunc",
	Doc:              "gathers a list of function signatures whose invocations may pass a pointer to a function that starts a goroutine",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, packid.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

// Result is the result of the nogofunc analyzer.
type Result struct {
	// AsyncSignatures is a set of signatures from which a goroutine can be reached in the callgraph
	// via functions that accept pointer arguments.
	AsyncSignatures map[callgraph.Signature]struct{}
	// ContainsGoStmt is a set of signatures whose declarations contain a go statement.
	ContainsGoStmt map[callgraph.Signature]struct{}
}

type signatureFacts struct {
	callgraph.DeclaredSignature
	StartsGoroutine bool
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
	}

	// nogofunc finds a list of functions declared in the target repository which don't start any
	// goroutines on their own. Calling into third-party code can be ignored
	sigByPos := make(map[token.Pos]*signatureFacts)
	for _, sig := range graph.PtrSignatures {
		sigByPos[sig.Pos] = &signatureFacts{sig, false}
	}

	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push { // this is called twice, once before and after the current node is added to the stack
			return true
		}
		switch n.(type) {
		case *ast.GoStmt: // goroutine here could be nested inside a function literal; we count it anyway.
			outerFunc := outermostFuncDecl(stack)
			if outerFunc != nil && sigByPos[outerFunc.Pos()] != nil {
				stats.AddCount(stats.StatPtrFuncStartsGoroutine, 1)
				sigByPos[outerFunc.Pos()].StartsGoroutine = true
			}
		}
		return true
	})

	result.ContainsGoStmt, result.AsyncSignatures = findAsyncSignatures(sigByPos, graph.ApproxCallGraph)
	return &result, nil
}

// findAsyncSignatures finds a list of Signatures for functions which eventually call a goroutine via some path
// of functions in the callgraph that can pass pointer arguments.
func findAsyncSignatures(sigs map[token.Pos]*signatureFacts, graph *callgraph.CallGraph) (map[callgraph.Signature]struct{}, map[callgraph.Signature]struct{}) {
	var toCheck []callgraph.Signature
	startsGoroutine := make(map[callgraph.Signature]struct{})
	for _, sig := range sigs {
		if sig.StartsGoroutine {
			startsGoroutine[sig.Signature] = struct{}{}
		} else {
			toCheck = append(toCheck, sig.Signature)
		}
	}
	// run a BFS over the called-by graph starting from the functions which start goroutines. Any function they
	// are called by is also marked as unsafe.
	unsafeRoots := make([]callgraph.Signature, 0, len(startsGoroutine))
	for sig := range startsGoroutine {
		unsafeRoots = append(unsafeRoots, sig)
	}

	asyncSignatures := make(map[callgraph.Signature]struct{})
	graph.CalledByBFS(unsafeRoots, func(sig callgraph.Signature) {
		asyncSignatures[sig] = struct{}{}
	})
	return startsGoroutine, asyncSignatures
}

func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}
