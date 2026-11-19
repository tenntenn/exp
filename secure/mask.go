package secure

import "strings"

// MaskFunc makes the masked text for a value. It gets the value, so the mask can
// use it (for example, to show only the last few characters).
type MaskFunc[V any] func(v V) string

// MaskDefault returns a MaskFunc that always returns n copies of char and
// ignores the value.
func MaskDefault[V any](char rune, n int) MaskFunc[V] {
	mask := strings.Repeat(string(char), n)
	return func(V) string {
		return mask
	}
}
