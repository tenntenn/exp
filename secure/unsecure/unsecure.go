// Package unsecure gives clear, easy-to-review access to the value wrapped by a
// secure.Value. Importing it makes an on-purpose unwrap easy to see in review,
// and the securecheck analyzer flags direct secure.Value.Raw calls while
// skipping the ones in this package.
package unsecure

import "github.com/tenntenn/exp/secure"

// Value returns the underlying value of v. Use it only where showing the secret
// value is intended.
func Value[V any](v secure.Value[V]) V {
	return v.Raw()
}
