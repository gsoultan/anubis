// Command testcleanup finds test cleanups that cannot run, and cleanups that
// cannot say they failed. scripts/check/test-cleanup.sh runs it.
//
// t.Cleanup runs AFTER the test function returns — after its defers. So a
// test that opens a pool, writes `defer pool.Close()` and registers a cleanup
// that deletes its fixture through that pool deletes nothing: the pool is shut
// by the time the cleanup runs. With the error discarded (`_, _ = pool.Exec`)
// nothing says so, and every run leaves its rows behind.
//
// It happened three times before this existed. identity_directory_test.go
// left a planted tenant in the dev database on its first run and was fixed
// with a comment. operators/reset_test.go and reseal/reseal_test.go were not:
// by 2026-09-26 the dev database held 79 leftover operators among 81, ten
// probe tenants and ten test signing keys.
//
// Two rules, both per function and both purely syntactic:
//
//   - A pool or connection opened in a test is closed with
//     t.Cleanup(x.Close), never defer. Cleanups run last-in-first-out, so a
//     close registered right after the open runs after every cleanup
//     registered later — including ones a helper registers with the pool it
//     was handed, which no per-function check could follow.
//   - A cleanup does not discard an Exec error. Report it with t.Errorf: a
//     fixture left behind is a failure, just a late one.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// opens names the calls whose result must not be closed with defer in a test.
var opens = map[string]bool{
	"pgxpool.New":           true,
	"pgxpool.NewWithConfig": true,
	"pgx.Connect":           true,
	"pgx.ConnectConfig":     true,
	"sql.Open":              true,
}

func main() {
	var findings []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "graphify-out":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := check(path)
		findings = append(findings, found...)
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "testcleanup:", err)
		os.Exit(2)
	}
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}

func check(path string) ([]string, error) {
	return checkSource(path, nil)
}

// checkSource parses src, or the file at filename when src is nil.
func checkSource(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	report := func(n ast.Node, msg string) {
		out = append(out, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), msg))
	}

	// Each function body is its own scope, closures included: one inside a
	// t.Run closure has exactly the bug this looks for. The body of a cleanup
	// is the exception — it is the last thing to run, so a defer there closes
	// its own resource after its own use.
	var visit func(body *ast.BlockStmt, isCleanup bool)
	visit = func(body *ast.BlockStmt, isCleanup bool) {
		opened := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				visit(x.Body, false)
				return false
			case *ast.CallExpr:
				if cleanup := cleanupFunc(x); cleanup != nil {
					for _, bad := range discardedExecs(cleanup) {
						report(bad, "cleanup discards an Exec error; report it with t.Errorf "+
							"so a fixture left behind fails the test")
					}
					visit(cleanup, true)
					return false
				}
			case *ast.AssignStmt:
				if len(x.Rhs) == 1 && opens[callName(x.Rhs[0])] {
					if id, ok := x.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
						opened[id.Name] = true
					}
				}
			case *ast.DeferStmt:
				if isCleanup {
					break
				}
				if sel, ok := x.Call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Close" {
					if id, ok := sel.X.(*ast.Ident); ok && opened[id.Name] {
						report(x, fmt.Sprintf("defer %s.Close() runs before every t.Cleanup; "+
							"close it from a t.Cleanup registered right after opening it", id.Name))
					}
				}
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			visit(fn.Body, false)
		}
	}
	return out, nil
}

// callName renders pkg.Func for a call on a package-qualified function.
func callName(e ast.Expr) string {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return pkg.Name + "." + sel.Sel.Name
}

// cleanupFunc returns the body of the function literal passed to x.Cleanup.
func cleanupFunc(call *ast.CallExpr) *ast.BlockStmt {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Cleanup" || len(call.Args) != 1 {
		return nil
	}
	lit, ok := call.Args[0].(*ast.FuncLit)
	if !ok {
		return nil
	}
	return lit.Body
}

// discardedExecs finds Exec calls whose error nobody looks at: assigned only
// to blanks, or called as a bare statement.
func discardedExecs(body *ast.BlockStmt) []ast.Node {
	var out []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false // a nested closure is somebody else's cleanup
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 && isExec(x.Rhs[0]) && allBlank(x.Lhs) {
				out = append(out, x)
			}
		case *ast.ExprStmt:
			if isExec(x.X) {
				out = append(out, x)
			}
		}
		return true
	})
	return out
}

func isExec(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && (sel.Sel.Name == "Exec" || sel.Sel.Name == "ExecContext")
}

func allBlank(lhs []ast.Expr) bool {
	for _, e := range lhs {
		if id, ok := e.(*ast.Ident); !ok || id.Name != "_" {
			return false
		}
	}
	return true
}
