package securecheck_test

import (
	"testing"

	"github.com/tenntenn/exp/secure/securecheck"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()

	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, securecheck.Analyzer, "a")
}
