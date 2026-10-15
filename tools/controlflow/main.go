package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

/*
Command controlflow enforces the repository control-flow invariants specified
in AGENTS.md:
  - Guard clauses and early returns.
  - No 'else' blocks.
*/
func main() {
	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"."}
	}

	fset := token.NewFileSet()
	var violations []string

	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error stating %s: %v\n", target, err)
			os.Exit(1)
		}

		if !info.IsDir() {
			scanFile(fset, target, &violations)
			continue
		}

		err = filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(path, ".go") || strings.Contains(path, "generated") {
				return nil
			}

			scanFile(fset, path, &violations)
			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "error scanning %s: %v\n", target, err)
			os.Exit(1)
		}
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "AGENTS.md Control Flow Violations (%d found):\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v)
		}
		os.Exit(1)
	}
}

func scanFile(fset *token.FileSet, path string, violations *[]string) {
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if stmt, ok := n.(*ast.IfStmt); ok && stmt.Else != nil {
			pos := fset.Position(stmt.Else.Pos())
			*violations = append(*violations, fmt.Sprintf("%s: forbidden 'else' branch (guard clauses and early returns required)", pos))
		}
		return true
	})
}
