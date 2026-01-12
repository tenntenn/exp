// +build js,wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/tenntenn/exp/backend/model"
	"github.com/tenntenn/exp/backend/parser"
)

func main() {
	c := make(chan struct{}, 0)

	// Register parse function
	js.Global().Set("goParse", js.FuncOf(parseWrapper))

	println("Go WASM module loaded successfully")

	<-c
}

// parseWrapper wraps the parse function for JavaScript
func parseWrapper(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{
			"error": "code parameter is required",
		}
	}

	code := args[0].String()
	format := "single"
	if len(args) >= 2 {
		format = args[1].String()
	}

	// Parse based on format
	var response *model.ParseResponse

	switch format {
	case "single":
		astNode, fset, file, astErrors := parser.ParseAST(code)

		var ssaFunctions []*model.SSAFunction
		var ssaErrors []model.ParseError

		if file != nil {
			ssaFunctions, ssaErrors = parser.BuildSSA(fset, file)
		}

		allErrors := append(astErrors, ssaErrors...)

		response = &model.ParseResponse{
			AST:    astNode,
			SSA:    ssaFunctions,
			Errors: allErrors,
		}

	case "txtar":
		astNode, ssaFunctions, files, errors := parser.ParseTxtar(code)

		response = &model.ParseResponse{
			AST:    astNode,
			SSA:    ssaFunctions,
			Files:  files,
			Errors: errors,
		}

	default:
		return map[string]interface{}{
			"error": "invalid format parameter",
		}
	}

	// Convert response to JSON
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	// Parse JSON string to JavaScript object
	return js.Global().Get("JSON").Call("parse", string(jsonBytes))
}
