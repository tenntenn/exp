// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package difftest supplies a set of tests that will operate on any
// implementation of a diff algorithm as exposed by
// "github.com/tenntenn/exp/toolsinternal/diff"
package difftest_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tenntenn/exp/toolsinternal/diff/difftest"
	"github.com/tenntenn/exp/toolsinternal/testenv"
)

func TestVerifyUnified(t *testing.T) {
	testenv.NeedsTool(t, "diff")
	for _, test := range difftest.TestCases {
		t.Run(test.Name, func(t *testing.T) {
			if test.NoDiff {
				t.Skip("diff tool produces expected different results")
			}
			diff, err := getDiffOutput(test.In, test.Out)
			if err != nil {
				t.Fatal(err)
			}
			if len(diff) > 0 {
				diff = difftest.UnifiedPrefix + diff
			}
			if diff != test.Unified {
				t.Errorf("unified:\n%s\ndiff -u:\n%s", test.Unified, diff)
			}
		})
	}
}

func getDiffOutput(a, b string) (string, error) {
	fileA, err := os.CreateTemp("", "myers.in")
	if err != nil {
		return "", err
	}
	defer os.Remove(fileA.Name())
	if _, err := fileA.Write([]byte(a)); err != nil {
		return "", err
	}
	if err := fileA.Close(); err != nil {
		return "", err
	}
	fileB, err := os.CreateTemp("", "myers.in")
	if err != nil {
		return "", err
	}
	defer os.Remove(fileB.Name())
	if _, err := fileB.Write([]byte(b)); err != nil {
		return "", err
	}
	if err := fileB.Close(); err != nil {
		return "", err
	}
	cmd := exec.Command("diff", "-u", fileA.Name(), fileB.Name())
	cmd.Env = append(cmd.Env, "LANG=en_US.UTF-8")
	out, err := cmd.Output()
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			return "", fmt.Errorf("can't exec %s: %v", cmd, err)
		}
		if len(out) == 0 {
			// Nonzero exit with no output: terminated by signal?
			return "", fmt.Errorf("%s failed: %v; stderr:\n%s", cmd, err, exit.Stderr)
		}
		// nonzero exit + output => files differ
	}
	diff := string(out)
	if len(diff) <= 0 {
		return diff, nil
	}
	bits := strings.SplitN(diff, "\n", 3)
	if len(bits) != 3 {
		return "", fmt.Errorf("diff output did not have file prefix:\n%s", diff)
	}
	return bits[2], nil
}
