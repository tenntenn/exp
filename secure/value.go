package secure

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
)

// aliases for Value[V]
type (
	String = Value[string]
)

// Value wraps a secret value of type V so it is not leaked by accident. Its
// String, GoString and log forms show a mask from its MaskFunc instead of the
// value; MarshalJSON keeps the real value so it can be read back. Use Raw to get
// the real value on purpose.
//
// The value is kept inside a closure, not a plain field, so reflection-based
// dumps such as cmp.Diff or fmt's %+v/%#v cannot read it while showing a
// mismatch.
type Value[V any] struct {
	get   func() V
	mask  MaskFunc[V]
	equal EqualFunc[V]

	// plain holds the raw value only while tests run (debugPlain), so
	// reflection-based diff tools like cmp.Diff can show it for debugging. In
	// production it stays the zero value and the raw value lives only in get,
	// where reflection cannot reach it.
	plain V
}

func _[V any]() {
	var _ interface {
		json.Marshaler
		json.Unmarshaler
		fmt.Stringer
		fmt.GoStringer
		slog.LogValuer
		sql.Scanner
		driver.Valuer
	} = &Value[V]{}
}

// debugPlain tells whether New copies the raw value into Value.plain for
// reflection-based debugging (e.g. cmp.Diff). It is true only under `go test`,
// so production never copies the value. Tests do not change it; they call the
// internal newValue (through the NewValue test helper) to pick either path, so
// they stay safe to run in parallel.
var debugPlain = testing.Testing()

// New makes a Value that wraps v. By default the value is masked by
// MaskDefault('*', 4); pass options such as WithMask to change that.
func New[V any](v V, opts ...Option[V]) Value[V] {
	return newValue(v, debugPlain, opts...)
}

// newValue builds a Value and copies the raw value into plain only when
// fillPlain is set. New passes debugPlain; tests pass an explicit value through
// the NewValue helper so they can cover both paths without sharing global state.
func newValue[V any](v V, fillPlain bool, opts ...Option[V]) Value[V] {
	val := Value[V]{
		get:  func() V { return v },
		mask: MaskDefault[V]('*', 4),
	}
	if fillPlain {
		val.plain = v
	}

	for _, opt := range opts {
		opt.apply(&val)
	}

	return val
}

func (v *Value[V]) setMask(mask MaskFunc[V]) {
	v.mask = mask
}

func (v *Value[V]) setEqual(equal EqualFunc[V]) {
	v.equal = equal
}

func (v *Value[V]) setRaw(raw V) {
	v.get = func() V { return raw }
	if debugPlain {
		v.plain = raw
	}
}

// raw returns the underlying value, or the zero value of V when none is set
// (e.g. on a zero Value).
func (v Value[V]) raw() V {
	if v.get == nil {
		var zero V
		return zero
	}
	return v.get()
}

// Raw returns the underlying secret value.
func (v Value[V]) Raw() V {
	return v.raw()
}

// masked returns the masked text. It uses the default mask when no MaskFunc is
// set (e.g. on a zero Value or after Scan/UnmarshalJSON).
func (v Value[V]) masked() string {
	mask := v.mask
	if mask == nil {
		mask = MaskDefault[V]('*', 4)
	}
	return mask(v.raw())
}

// MarshalJSON implements json.Marshaler. It writes the underlying value so it
// can be read back by UnmarshalJSON. Only String, GoString and LogValue mask.
func (v Value[V]) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.raw())
}

// UnmarshalJSON implements json.Unmarshaler. It reads data into the underlying
// value.
func (v *Value[V]) UnmarshalJSON(data []byte) error {
	var raw V
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.setRaw(raw)
	return nil
}

// String implements fmt.Stringer. It returns the mask.
func (v Value[V]) String() string {
	return v.masked()
}

// GoString implements fmt.GoStringer so the %#v verb returns the mask instead of
// the underlying value.
func (v Value[V]) GoString() string {
	return v.masked()
}

// LogValue implements slog.LogValuer. It returns the mask so the secret value is
// not written to logs.
func (v Value[V]) LogValue() slog.Value {
	return slog.StringValue(v.masked())
}

// Scan implements sql.Scanner. If the underlying value type implements
// sql.Scanner, it is used; otherwise src is stored when it can be assigned to V.
func (v *Value[V]) Scan(src any) error {
	var raw V
	if s, ok := any(&raw).(sql.Scanner); ok {
		if err := s.Scan(src); err != nil {
			return err
		}
		v.setRaw(raw)
		return nil
	}

	sv, ok := src.(V)
	if !ok {
		return fmt.Errorf("type assert error %T to %T", src, raw)
	}
	v.setRaw(sv)

	return nil
}

// Value implements driver.Valuer so the wrapped value can be used as a SQL
// parameter. If the underlying value type implements driver.Valuer, it is used;
// otherwise the standard parameter conversion is applied.
func (v Value[V]) Value() (driver.Value, error) {
	return driver.DefaultParameterConverter.ConvertValue(v.raw())
}

// Equal tells whether v and o hold equal underlying values. The mask is not
// checked: it is only for display (and a func, which cannot be compared). Each
// side compares in this order: the EqualFunc set with WithEqual, an Equal(V)
// bool method on the value type, and finally reflect.DeepEqual.
//
// Both sides run their comparison and the results are combined with AND, so the
// values are equal only when both agree. This keeps Equal the same in either
// direction even when v and o use different EqualFuncs, which
// github.com/google/go-cmp/cmp needs (it would otherwise panic). go-cmp also
// uses Equal to compare values with unexported fields, so importers do not need
// a go-cmp dependency.
func (v Value[V]) Equal(o Value[V]) bool {
	a, b := v.raw(), o.raw()
	return v.equalValue(a, b) && o.equalValue(b, a)
}

// equalValue compares a and b using v's chosen way to compare.
func (v Value[V]) equalValue(a, b V) bool {
	if v.equal != nil {
		return v.equal(a, b)
	}
	if eq, ok := any(a).(interface{ Equal(V) bool }); ok {
		return eq.Equal(b)
	}
	return reflect.DeepEqual(a, b)
}

// IsZero tells whether the underlying value is its zero value. If the value type
// has an IsZero() bool method, it is used; otherwise the value is compared with
// its zero value using reflect.DeepEqual. This lets Value work with
// encoding/json's "omitzero" option.
func (v Value[V]) IsZero() bool {
	raw := v.raw()
	if z, ok := any(raw).(interface{ IsZero() bool }); ok {
		return z.IsZero()
	}
	var zero V
	return reflect.DeepEqual(raw, zero)
}
