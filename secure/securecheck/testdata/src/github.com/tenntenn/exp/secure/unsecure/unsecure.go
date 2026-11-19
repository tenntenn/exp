// Package unsecure is a small stub used by the securecheck analysistest
// fixtures. Its Raw call must be ignored by the analyzer.
package unsecure

import "github.com/tenntenn/exp/secure"

func Value[V any](v secure.Value[V]) V {
	return v.Raw()
}
