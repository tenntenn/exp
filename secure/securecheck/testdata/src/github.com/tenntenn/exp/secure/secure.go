// Package secure is a small stub of github.com/tenntenn/exp/secure used by the
// securecheck analysistest fixtures.
package secure

type Value[V any] struct {
	v V
}

func New[V any](v V) Value[V] {
	return Value[V]{v: v}
}

func (v Value[V]) Raw() V {
	return v.v
}

func (v Value[V]) String() string {
	return "****"
}
