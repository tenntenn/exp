package secure

// NewValue builds a Value and lets the caller choose whether the raw value is
// copied into the plain field, so tests can cover both the production
// (fillPlain=false) and debugging (fillPlain=true) paths in parallel without
// sharing global state.
func NewValue[V any](v V, fillPlain bool, opts ...Option[V]) Value[V] {
	return newValue(v, fillPlain, opts...)
}
