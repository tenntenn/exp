package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/tenntenn/exp/backend/api"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

//go:embed frontend/dist
var frontendFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Create Connect RPC handler
	handler := api.NewParserServiceHandler()

	// Register Connect RPC endpoint
	path, connectHandler := newParserServiceHandler(handler)
	mux.Handle(path, connectHandler)

	// Serve frontend
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve from embedded frontend
		distFS, err := fs.Sub(frontendFS, "frontend/dist")
		if err == nil {
			http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
		} else {
			// Fallback to simple message
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Go AST/SSA Visualizer</title>
</head>
<body>
    <h1>Go AST/SSA Visualizer</h1>
    <p>Connect RPC Server is running. Frontend is not yet built.</p>
    <p>RPC Endpoint: <code>POST /parser.v1.ParserService/Parse</code></p>
</body>
</html>`)
		}
	})

	addr := ":" + port
	log.Printf("Server starting on http://localhost%s", addr)

	// Use h2c to support HTTP/2 without TLS
	if err := http.ListenAndServe(addr, h2c.NewHandler(corsMiddleware(mux), &http2.Server{})); err != nil {
		log.Fatal(err)
	}
}

// newParserServiceHandler creates a Connect RPC handler for ParserService
func newParserServiceHandler(handler *api.ParserServiceHandler) (string, http.Handler) {
	path := "/parser.v1.ParserService/Parse"
	connectHandler := connect.NewUnaryHandler(
		path,
		handler.Parse,
		connect.WithCodec(&api.ParseRequestCodec{}),
	)
	return path, connectHandler
}

// corsMiddleware adds CORS headers for development
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Connect-Protocol-Version")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
