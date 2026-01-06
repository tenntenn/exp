package api

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"
	"github.com/tenntenn/exp/backend/model"
	"github.com/tenntenn/exp/backend/parser"
)

// ParserServiceHandler implements the Connect RPC ParserService
type ParserServiceHandler struct{}

// NewParserServiceHandler creates a new ParserServiceHandler
func NewParserServiceHandler() *ParserServiceHandler {
	return &ParserServiceHandler{}
}

// Parse handles the Parse RPC method
func (h *ParserServiceHandler) Parse(
	ctx context.Context,
	req *connect.Request[model.ParseRequest],
) (*connect.Response[model.ParseResponse], error) {
	// Validate request
	if req.Msg.Code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	// Default format to "single" if not specified
	if req.Msg.Format == "" {
		req.Msg.Format = "single"
	}

	// For MVP, we only support single file format
	if req.Msg.Format != "single" {
		return nil, connect.NewError(
			connect.CodeUnimplemented,
			nil,
		)
	}

	// Parse AST
	astNode, fset, file, astErrors := parser.ParseAST(req.Msg.Code)

	// Build SSA
	var ssaFunctions []*model.SSAFunction
	var ssaErrors []model.ParseError

	if file != nil {
		ssaFunctions, ssaErrors = parser.BuildSSA(fset, file)
	}

	// Combine errors
	allErrors := append(astErrors, ssaErrors...)

	// Build response
	response := &model.ParseResponse{
		AST:    astNode,
		SSA:    ssaFunctions,
		Errors: allErrors,
	}

	return connect.NewResponse(response), nil
}

// ParseRequestCodec implements custom JSON codec for ParseRequest
type ParseRequestCodec struct{}

func (c *ParseRequestCodec) Name() string {
	return "json"
}

func (c *ParseRequestCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *ParseRequestCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
