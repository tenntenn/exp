package secure

// Option changes a Value made by New.
type Option[V any] interface {
	apply(*Value[V])
}

type optionFunc[V any] func(v *Value[V])

func (f optionFunc[V]) apply(v *Value[V]) { f(v) }

// WithMask sets the MaskFunc used to make the masked text returned by String and
// LogValue.
func WithMask[V any](mask MaskFunc[V]) Option[V] {
	return optionFunc[V](func(v *Value[V]) {
		v.setMask(mask)
	})
}

// WithEqual sets the EqualFunc used by Value.Equal to compare underlying values.
// It is used before an Equal method on V and the reflect.DeepEqual fallback.
func WithEqual[V any](equal EqualFunc[V]) Option[V] {
	return optionFunc[V](func(v *Value[V]) {
		v.setEqual(equal)
	})
}
