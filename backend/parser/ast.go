package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"

	"github.com/tenntenn/exp/backend/model"
)

// ParseAST parses Go source code and returns an AST tree
func ParseAST(src string) (*model.ASTNode, *token.FileSet, *ast.File, []model.ParseError) {
	fset := token.NewFileSet()
	var errors []model.ParseError

	// Parse the source code
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		errors = append(errors, model.ParseError{
			Message:  err.Error(),
			Position: model.Position{Line: 1, Column: 1, Offset: 0},
			Severity: "error",
		})
		return nil, fset, nil, errors
	}

	// Convert AST to our model
	astNode := convertASTNode(fset, file)
	return astNode, fset, file, errors
}

// convertASTNode converts a go/ast.Node to our model.ASTNode
func convertASTNode(fset *token.FileSet, node ast.Node) *model.ASTNode {
	if node == nil {
		return nil
	}

	result := &model.ASTNode{
		Type:  getNodeType(node),
		Start: positionFromPos(fset, node.Pos()),
		End:   positionFromPos(fset, node.End()),
	}

	// Add node-specific information and children
	switch n := node.(type) {
	case *ast.File:
		result.Value = map[string]interface{}{
			"name": n.Name.Name,
		}
		for _, decl := range n.Decls {
			if child := convertASTNode(fset, decl); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.FuncDecl:
		funcInfo := map[string]interface{}{
			"name": n.Name.Name,
		}
		if n.Recv != nil && len(n.Recv.List) > 0 {
			funcInfo["receiver"] = true
		}
		result.Value = funcInfo

		if n.Type != nil {
			if child := convertASTNode(fset, n.Type); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if n.Body != nil {
			if child := convertASTNode(fset, n.Body); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.FuncType:
		if n.Params != nil {
			if child := convertASTNode(fset, n.Params); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if n.Results != nil {
			if child := convertASTNode(fset, n.Results); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.FieldList:
		for _, field := range n.List {
			if child := convertASTNode(fset, field); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.Field:
		if len(n.Names) > 0 {
			names := make([]string, len(n.Names))
			for i, name := range n.Names {
				names[i] = name.Name
			}
			result.Value = map[string]interface{}{"names": names}
		}
		if n.Type != nil {
			if child := convertASTNode(fset, n.Type); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.BlockStmt:
		for _, stmt := range n.List {
			if child := convertASTNode(fset, stmt); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.ExprStmt:
		if child := convertASTNode(fset, n.X); child != nil {
			result.Children = append(result.Children, child)
		}

	case *ast.CallExpr:
		if child := convertASTNode(fset, n.Fun); child != nil {
			result.Children = append(result.Children, child)
		}
		for _, arg := range n.Args {
			if child := convertASTNode(fset, arg); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.SelectorExpr:
		if child := convertASTNode(fset, n.X); child != nil {
			result.Children = append(result.Children, child)
		}
		result.Value = map[string]interface{}{"sel": n.Sel.Name}

	case *ast.Ident:
		result.Value = map[string]interface{}{"name": n.Name}

	case *ast.BasicLit:
		result.Value = map[string]interface{}{
			"kind":  n.Kind.String(),
			"value": n.Value,
		}

	case *ast.ReturnStmt:
		for _, expr := range n.Results {
			if child := convertASTNode(fset, expr); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.AssignStmt:
		result.Value = map[string]interface{}{"tok": n.Tok.String()}
		for _, lhs := range n.Lhs {
			if child := convertASTNode(fset, lhs); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		for _, rhs := range n.Rhs {
			if child := convertASTNode(fset, rhs); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.IfStmt:
		if n.Init != nil {
			if child := convertASTNode(fset, n.Init); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if child := convertASTNode(fset, n.Cond); child != nil {
			result.Children = append(result.Children, child)
		}
		if child := convertASTNode(fset, n.Body); child != nil {
			result.Children = append(result.Children, child)
		}
		if n.Else != nil {
			if child := convertASTNode(fset, n.Else); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.ForStmt:
		if n.Init != nil {
			if child := convertASTNode(fset, n.Init); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if n.Cond != nil {
			if child := convertASTNode(fset, n.Cond); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if n.Post != nil {
			if child := convertASTNode(fset, n.Post); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		if child := convertASTNode(fset, n.Body); child != nil {
			result.Children = append(result.Children, child)
		}

	case *ast.BinaryExpr:
		result.Value = map[string]interface{}{"op": n.Op.String()}
		if child := convertASTNode(fset, n.X); child != nil {
			result.Children = append(result.Children, child)
		}
		if child := convertASTNode(fset, n.Y); child != nil {
			result.Children = append(result.Children, child)
		}

	case *ast.GenDecl:
		result.Value = map[string]interface{}{"tok": n.Tok.String()}
		for _, spec := range n.Specs {
			if child := convertASTNode(fset, spec); child != nil {
				result.Children = append(result.Children, child)
			}
		}

	case *ast.ImportSpec:
		value := make(map[string]interface{})
		if n.Name != nil {
			value["name"] = n.Name.Name
		}
		if n.Path != nil {
			value["path"] = n.Path.Value
		}
		result.Value = value

	case *ast.ValueSpec:
		if len(n.Names) > 0 {
			names := make([]string, len(n.Names))
			for i, name := range n.Names {
				names[i] = name.Name
			}
			result.Value = map[string]interface{}{"names": names}
		}
		if n.Type != nil {
			if child := convertASTNode(fset, n.Type); child != nil {
				result.Children = append(result.Children, child)
			}
		}
		for _, value := range n.Values {
			if child := convertASTNode(fset, value); child != nil {
				result.Children = append(result.Children, child)
			}
		}
	}

	return result
}

// getNodeType returns the type name of an AST node
func getNodeType(node ast.Node) string {
	t := reflect.TypeOf(node)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// positionFromPos converts a token.Pos to our model.Position
func positionFromPos(fset *token.FileSet, pos token.Pos) model.Position {
	if !pos.IsValid() {
		return model.Position{}
	}
	position := fset.Position(pos)
	return model.Position{
		Line:   position.Line,
		Column: position.Column,
		Offset: position.Offset,
	}
}
