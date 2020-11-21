// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// A modified version of the code found in the Golang standard library which handles nested loops.
package loopclosure

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const Doc = `This is an augmented version of the loopanalyzer found in the
standard library. It handles nested loops and avoids relying on type-checking
info.`

var Analyzer = &analysis.Analyzer{
	Name:     "loopclosure-augmented",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.RangeStmt)(nil),
	}
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		inspectBody(n, nil, pass)
	})
	return nil, nil
}

type LoopVar struct {
	ident *ast.Ident
	body  *ast.RangeStmt
}

func inspectBody(n ast.Node, outerVars []LoopVar, pass *analysis.Pass) {
	loopVars := make([]LoopVar, len(outerVars))
	copy(loopVars, outerVars)

	// Find the variables updated by the loop statement.
	addVar := func(expr ast.Expr, body *ast.RangeStmt) {
		if id, ok := expr.(*ast.Ident); ok {
			loopVars = append(loopVars, LoopVar{
				ident: id,
				body:  body,
			})
		}
	}
	var body *ast.BlockStmt
	switch n := n.(type) {
	case *ast.RangeStmt:
		body = n.Body
		addVar(n.Key, n)
		addVar(n.Value, n)
	// Keep checking the contents of nested blocks, but only capture range loop variables as targets
	case *ast.ForStmt:
		body = n.Body
	case *ast.IfStmt:
		body = n.Body
	case *ast.SwitchStmt:
		body = n.Body
	}
	if len(loopVars) == 0 {
		return
	}

	if body == nil || len(body.List) == 0 {
		return
	}
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.GoStmt:
			if lit, ok := s.Call.Fun.(*ast.FuncLit); ok {
				ast.Inspect(lit.Body, findLoopVar(loopVars, pass))
			}
		case *ast.DeferStmt:
			if lit, ok := s.Call.Fun.(*ast.FuncLit); ok {
				ast.Inspect(lit.Body, findLoopVar(loopVars, pass))
			}

		// recurse into nested loops as well and perform the same check.
		case *ast.RangeStmt:
			inspectBody(s, loopVars, pass)
		case *ast.ForStmt:
			inspectBody(s, loopVars, pass)
		case *ast.IfStmt:
			inspectBody(s, loopVars, pass)
		case *ast.SwitchStmt:
			inspectBody(s, loopVars, pass)
		}
	}
}

func findLoopVar(loopVars []LoopVar, pass *analysis.Pass) func(ast.Node) bool {
	return func(n ast.Node) bool {
		switch id := n.(type) {
		case *ast.Ident:
			if id.Obj != nil && id.Obj.Kind != ast.Var {
				// Identifier is not referring to a variable
				return true
			}
			for _, v := range loopVars {
				if v.ident.Obj == id.Obj {
					pass.ReportRangef(v.body, "loop variable %s captured by func literal",
						id.Name)
				}
			}
		case *ast.KeyValueExpr:
			ast.Inspect(id.Value, findLoopVar(loopVars, pass))
			return false
		}
		return true
	}
}
