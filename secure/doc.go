// Package secure wraps a secret value so it is not leaked by accident.
//
// A [Value] keeps its underlying value of type V behind a mask. [Value.String],
// [Value.GoString], the fmt verbs and [Value.LogValue] all return the mask
// instead of the value, so logging or formatting a Value never shows it. The
// value is kept inside a closure, so reflection-based dumps such as cmp.Diff or
// fmt's %+v cannot reach it.
//
// Get the underlying value on purpose with [Value.Raw]. Because that removes the
// protection, the securecheck analyzer flags direct Raw calls; get the value
// through [github.com/tenntenn/exp/secure/unsecure.Value] instead, which makes
// the unwrap easy to see in review.
//
// Note that [Value.MarshalJSON] writes the underlying value (not the mask) so it
// can be read back by [Value.UnmarshalJSON]; only the human-facing forms are
// masked.
package secure
