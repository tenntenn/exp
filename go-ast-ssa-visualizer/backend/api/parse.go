package api

import (
	"encoding/json"
	"net/http"

	"github.com/tenntenn/exp/backend/model"
	"github.com/tenntenn/exp/backend/parser"
)

// ParseHandler handles the /api/parse endpoint
func ParseHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req model.ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	// Default format to "single" if not specified
	if req.Format == "" {
		req.Format = "single"
	}

	// For MVP, we only support single file format
	if req.Format != "single" {
		http.Error(w, "Only 'single' format is supported in MVP", http.StatusBadRequest)
		return
	}

	// Parse AST
	astNode, fset, file, astErrors := parser.ParseAST(req.Code)

	// Build SSA
	var ssaFunctions []*model.SSAFunction
	var ssaErrors []model.ParseError

	if file != nil {
		ssaFunctions, ssaErrors = parser.BuildSSA(fset, file)
	}

	// Combine errors
	allErrors := append(astErrors, ssaErrors...)

	// Build response
	response := model.ParseResponse{
		AST:    astNode,
		SSA:    ssaFunctions,
		Errors: allErrors,
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
