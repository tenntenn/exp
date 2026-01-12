#!/bin/bash

set -e

echo "Building WASM module..."

# Build WASM
GOOS=js GOARCH=wasm go build -o ../frontend/public/parser.wasm

# Copy wasm_exec.js from Go installation
GOROOT=$(go env GOROOT)
if [ -f "$GOROOT/lib/wasm/wasm_exec.js" ]; then
    cp "$GOROOT/lib/wasm/wasm_exec.js" ../frontend/public/
elif [ -f "$GOROOT/misc/wasm/wasm_exec.js" ]; then
    cp "$GOROOT/misc/wasm/wasm_exec.js" ../frontend/public/
else
    echo "Error: wasm_exec.js not found"
    exit 1
fi

echo "WASM module built successfully!"
echo "Output: frontend/public/parser.wasm"
echo "Helper: frontend/public/wasm_exec.js"
