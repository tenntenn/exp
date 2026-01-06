package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/tenntenn/exp/backend/api"
)

//go:embed frontend/dist
var frontendFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/parse", corsMiddleware(api.ParseHandler))

	// Serve frontend (will be built later)
	// For now, serve a simple test page
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
    <p>API Server is running. Frontend is not yet built.</p>
    <p>API Endpoint: <code>POST /api/parse</code></p>
</body>
</html>`)
		}
	})

	addr := ":" + port
	log.Printf("Server starting on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// corsMiddleware adds CORS headers for development
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
