package secure

// EqualFunc tells whether two underlying values are equal. Pass it to WithEqual
// to change how Value.Equal compares values, for example by wrapping
// github.com/google/go-cmp/cmp.Equal with cmp options.
type EqualFunc[V any] func(a, b V) bool
