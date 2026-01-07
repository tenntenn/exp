package api

import (
	"bytes"
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/tenntenn/goplayground"
)

// ShareRequest represents a request to share code on Go Playground
type ShareRequest struct {
	Code string `json:"code"`
}

// ShareResponse represents the response from sharing code
type ShareResponse struct {
	Hash string `json:"hash"`
	URL  string `json:"url"`
}

// LoadRequest represents a request to load code from Go Playground
type LoadRequest struct {
	Hash string `json:"hash"`
}

// LoadResponse represents the response from loading code
type LoadResponse struct {
	Code string `json:"code"`
}

// Share handles the Share RPC method
func (h *ParserServiceHandler) Share(
	ctx context.Context,
	req *connect.Request[ShareRequest],
) (*connect.Response[ShareResponse], error) {
	if req.Msg.Code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	// Create Go Playground client
	cli := &goplayground.Client{}

	// Share code to Go Playground
	shareURL, err := cli.Share(req.Msg.Code)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Extract hash from URL path (e.g., /p/HASH)
	hash := strings.TrimPrefix(shareURL.Path, "/p/")

	response := &ShareResponse{
		Hash: hash,
		URL:  shareURL.String(),
	}

	return connect.NewResponse(response), nil
}

// Load handles the Load RPC method
func (h *ParserServiceHandler) Load(
	ctx context.Context,
	req *connect.Request[LoadRequest],
) (*connect.Response[LoadResponse], error) {
	if req.Msg.Hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	// Create Go Playground client
	cli := &goplayground.Client{}

	// Download code from Go Playground
	var buf bytes.Buffer
	if err := cli.Download(&buf, req.Msg.Hash); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	response := &LoadResponse{
		Code: buf.String(),
	}

	return connect.NewResponse(response), nil
}
