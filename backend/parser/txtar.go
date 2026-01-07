package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"golang.org/x/tools/txtar"

	"github.com/tenntenn/exp/backend/model"
)

// ParseTxtar parses txtar format code containing multiple files
func ParseTxtar(txtarContent string) (*model.ASTNode, []*model.SSAFunction, []model.FileInfo, []model.ParseError) {
	var errors []model.ParseError
	var files []model.FileInfo

	// Parse txtar format
	archive := txtar.Parse([]byte(txtarContent))

	// Extract files from archive
	for _, file := range archive.Files {
		files = append(files, model.FileInfo{
			Name:    file.Name,
			Content: string(file.Data),
		})
	}

	if len(files) == 0 {
		errors = append(errors, model.ParseError{
			Message:  "No files found in txtar archive",
			Position: model.Position{Line: 1, Column: 1, Offset: 0},
			Severity: "error",
		})
		return nil, nil, files, errors
	}

	// Create file set and parse all files
	fset := token.NewFileSet()
	astFiles := make(map[string]*ast.File)
	var firstFile *ast.File

	for _, fileInfo := range files {
		file, err := parser.ParseFile(fset, fileInfo.Name, fileInfo.Content, parser.ParseComments)
		if err != nil {
			errors = append(errors, model.ParseError{
				Message:  fmt.Sprintf("%s: %v", fileInfo.Name, err),
				Position: model.Position{Line: 1, Column: 1, Offset: 0},
				Severity: "error",
			})
			continue
		}

		astFiles[fileInfo.Name] = file
		if firstFile == nil {
			firstFile = file
		}
	}

	if firstFile == nil {
		return nil, nil, files, errors
	}

	// Convert first file's AST (or we could merge all files into one tree)
	astNode := convertASTNode(fset, firstFile)

	// Build SSA for the package
	ssaFuncs, ssaErrors := buildSSAForPackage(fset, astFiles)
	errors = append(errors, ssaErrors...)

	return astNode, ssaFuncs, files, errors
}

// buildSSAForPackage builds SSA form for a package with multiple files
func buildSSAForPackage(fset *token.FileSet, astFiles map[string]*ast.File) ([]*model.SSAFunction, []model.ParseError) {
	var errors []model.ParseError

	// Type-check the package
	conf := types.Config{
		Importer: nil, // Use default importer
		Error: func(err error) {
			errors = append(errors, model.ParseError{
				Message:  err.Error(),
				Position: model.Position{Line: 1, Column: 1, Offset: 0},
				Severity: "error",
			})
		},
	}

	// Convert map to slice for type checker
	var astFilesList []*ast.File
	for _, f := range astFiles {
		astFilesList = append(astFilesList, f)
	}

	pkg, err := conf.Check("main", fset, astFilesList, nil)
	if err != nil && pkg == nil {
		// If type checking completely failed, return errors
		return nil, errors
	}

	// Build SSA
	prog := ssa.NewProgram(fset, ssa.SanityCheckFunctions)
	ssaPkg := prog.CreatePackage(pkg, astFilesList, &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}, false)

	ssaPkg.Build()

	// Extract all functions
	var functions []*model.SSAFunction
	for _, member := range ssaPkg.Members {
		if fn, ok := member.(*ssa.Function); ok {
			functions = append(functions, convertSSAFunction(fset, fn))
		}
	}

	// Also get functions from ssautil to ensure we get all functions
	allFuncs := ssautil.AllFunctions(prog)
	funcMap := make(map[*ssa.Function]bool)

	for fn := range allFuncs {
		if fn.Pkg == ssaPkg && !funcMap[fn] {
			funcMap[fn] = true
			// Check if not already added
			found := false
			for _, existing := range functions {
				if existing.Name == fn.Name() {
					found = true
					break
				}
			}
			if !found {
				functions = append(functions, convertSSAFunction(fset, fn))
			}
		}
	}

	return functions, errors
}
