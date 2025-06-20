// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// The pprof command prints the total time in a pprof profile provided
// through the standard input.
package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/tenntenn/exp/toolsinternal/pprof"
)

func main() {
	rd, err := gzip.NewReader(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	payload, err := io.ReadAll(rd)
	if err != nil {
		log.Fatal(err)
	}
	total, err := pprof.TotalTime(payload)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(total)
}
