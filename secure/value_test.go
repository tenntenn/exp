package secure_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tenntenn/exp/secure"
)

func TestValue_representation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value    string
		maskChar rune
		maskLen  int // 0 means the default mask
		wantRaw  string
		wantMask string
	}{
		"default mask": {"sample-secret", 0, 0, "sample-secret", "****"},
		"custom mask":  {"sample-secret", '#', 3, "sample-secret", "###"},
		"empty value":  {"", 0, 0, "", "****"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var opts []secure.Option[string]
			if tt.maskLen > 0 {
				opts = append(opts, secure.WithMask(secure.MaskDefault[string](tt.maskChar, tt.maskLen)))
			}
			v := secure.New(tt.value, opts...)

			if diff := cmp.Diff(tt.wantRaw, v.Raw()); diff != "" {
				t.Errorf("Raw() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantMask, v.String()); diff != "" {
				t.Errorf("String() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantMask, v.LogValue().String()); diff != "" {
				t.Errorf("LogValue() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValue_maskUsesValue(t *testing.T) {
	t.Parallel()

	mask := func(s string) string {
		if s == "" {
			return ""
		}
		return s[:1] + "***"
	}
	v := secure.New("sample-secret", secure.WithMask(mask))

	if diff := cmp.Diff("s***", v.String()); diff != "" {
		t.Errorf("String() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("sample-secret", v.Raw()); diff != "" {
		t.Errorf("Raw() mismatch (-want +got):\n%s", diff)
	}
}

func TestValue_slogDoesNotLeak(t *testing.T) {
	t.Parallel()

	const raw = "sample-secret"

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("login", "password", secure.New(raw))

	if bytes.Contains(buf.Bytes(), []byte(raw)) {
		t.Errorf("log output leaked the raw value: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("****")) {
		t.Errorf("log output missing mask: %s", buf.String())
	}
}

func TestValue_MarshalJSON(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(secure.New("sample-secret"))
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if diff := cmp.Diff(`"sample-secret"`, string(b)); diff != "" {
		t.Errorf("json output mismatch (-want +got):\n%s", diff)
	}
}

func TestValue_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := secure.New("sample-secret")

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var got secure.Value[string]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestValue_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var v secure.Value[string]
	if err := json.Unmarshal([]byte(`"sample-secret"`), &v); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if diff := cmp.Diff("sample-secret", v.Raw()); diff != "" {
		t.Errorf("Raw() mismatch (-want +got):\n%s", diff)
	}
	// After UnmarshalJSON there is no mask set, so it uses the default.
	if diff := cmp.Diff("****", v.String()); diff != "" {
		t.Errorf("String() mismatch (-want +got):\n%s", diff)
	}
}

func TestValue_Scan(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		src     any
		want    string
		wantErr bool
	}{
		"string source": {"sample-secret", "sample-secret", false},
		"type mismatch": {123, "", true},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var v secure.Value[string]
			err := v.Scan(tt.src)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Scan() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan() error: %v", err)
			}
			if diff := cmp.Diff(tt.want, v.Raw()); diff != "" {
				t.Errorf("Raw() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValue_ScanDelegatesToScanner(t *testing.T) {
	t.Parallel()

	var v secure.Value[sql.NullString]
	if err := v.Scan("sample-value"); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	want := sql.NullString{String: "sample-value", Valid: true}
	if diff := cmp.Diff(want, v.Raw()); diff != "" {
		t.Errorf("Raw() mismatch (-want +got):\n%s", diff)
	}
}

func TestValue_Value(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value string
		want  driver.Value
	}{
		"string source": {"sample-secret", "sample-secret"},
		"empty value":   {"", ""},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := secure.New(tt.value).Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Value() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValue_ValueDelegatesToValuer(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		ns   sql.NullString
		want driver.Value
	}{
		"valid": {sql.NullString{String: "sample-value", Valid: true}, "sample-value"},
		"null":  {sql.NullString{}, nil},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := secure.New(tt.ns).Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Value() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValue_Equal(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		a    string
		b    string
		want bool
	}{
		"same value":      {"sample-secret", "sample-secret", true},
		"different value": {"sample-secret1", "sample-secret2", false},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a, b := secure.New(tt.a), secure.New(tt.b)
			if got := a.Equal(b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
			if got := cmp.Equal(a, b); got != tt.want {
				t.Errorf("cmp.Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValue_EqualIgnoresMask(t *testing.T) {
	t.Parallel()

	a := secure.New("sample-secret")
	b := secure.New("sample-secret", secure.WithMask(secure.MaskDefault[string]('#', 3)))

	if !a.Equal(b) {
		t.Error("Equal() = false, want true (mask must not affect equality)")
	}
}

func TestValue_WithEqual(t *testing.T) {
	t.Parallel()

	// Compare without caring about case. Equal joins both sides with AND, so
	// both must use the case-insensitive EqualFunc for different cases to be
	// equal.
	a := secure.New("SAMPLE-SECRET", secure.WithEqual(strings.EqualFold))
	b := secure.New("sample-secret", secure.WithEqual(strings.EqualFold))

	if !a.Equal(b) {
		t.Error("Equal() = false, want true (WithEqual should compare case-insensitively)")
	}
	if !cmp.Equal(a, b) {
		t.Error("cmp.Equal() = false, want true")
	}
}

func TestValue_EqualSymmetricWithMixedEqual(t *testing.T) {
	t.Parallel()

	// Only one side has an EqualFunc. Equal must give the same result both ways,
	// and go-cmp must not panic on the mismatch.
	a := secure.New("sample-secret", secure.WithEqual(strings.EqualFold))
	b := secure.New("sample-secret")

	if a.Equal(b) != b.Equal(a) {
		t.Errorf("Equal is asymmetric: a.Equal(b)=%v, b.Equal(a)=%v", a.Equal(b), b.Equal(a))
	}
	if !cmp.Equal(a, b) {
		t.Error("cmp.Equal() = false, want true")
	}
}

// equaler treats two values as equal when their id matches, so noise is
// ignored. It checks that Value.Equal uses the value's own Equal method instead
// of reflect.DeepEqual.
type equaler struct {
	id    int
	noise string
}

func (e equaler) Equal(o equaler) bool { return e.id == o.id }

func TestValue_EqualUsesUnderlyingEqual(t *testing.T) {
	t.Parallel()

	a := secure.New(equaler{id: 1, noise: "sample-noise1"})
	b := secure.New(equaler{id: 1, noise: "sample-noise2"})

	if !a.Equal(b) {
		t.Error("Equal() = false, want true (underlying Equal should ignore noise)")
	}
	if !cmp.Equal(a, b) {
		t.Error("cmp.Equal() = false, want true")
	}
}

func TestValue_cmpDiffShowsRawInTests(t *testing.T) {
	t.Parallel()

	// With fillPlain set, the raw value is copied into the plain field, so
	// cmp.Diff can show it for debugging.
	a := secure.NewValue("sample-secret-AAA", true)
	b := secure.NewValue("sample-secret-BBB", true)

	diff := cmp.Diff(a, b)
	if !strings.Contains(diff, "AAA") || !strings.Contains(diff, "BBB") {
		t.Errorf("cmp.Diff should surface raw values during tests:\n%s", diff)
	}
}

func TestValue_cmpDiffHidesRawInProduction(t *testing.T) {
	t.Parallel()

	// fillPlain off matches production: the raw value lives only in the closure,
	// so nothing leaks through cmp.Diff.
	a := secure.NewValue("sample-secret-AAA", false)
	b := secure.NewValue("sample-secret-BBB", false)

	diff := cmp.Diff(a, b)
	if strings.Contains(diff, "AAA") || strings.Contains(diff, "BBB") {
		t.Errorf("cmp.Diff leaked the raw value in production mode:\n%s", diff)
	}
}

func TestValue_GoStringDoesNotLeak(t *testing.T) {
	t.Parallel()

	got := fmt.Sprintf("%#v", secure.New("sample-secret"))
	if strings.Contains(got, "sample-secret") {
		t.Errorf("%%#v leaked the raw value: %s", got)
	}
	if !strings.Contains(got, "****") {
		t.Errorf("%%#v missing mask: %s", got)
	}
}

func TestValue_VerbDoesNotLeak(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		verb string
	}{
		"%v":  {"%v"},
		"%+v": {"%+v"},
		"%#v": {"%#v"},
		"%s":  {"%s"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := fmt.Sprintf(tt.verb, secure.New("sample-secret"))
			if strings.Contains(got, "sample-secret") {
				t.Errorf("%s leaked the raw value: %s", tt.verb, got)
			}
		})
	}
}

func TestValue_IsZero(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value string
		want  bool
	}{
		"zero value":     {"", true},
		"non-zero value": {"sample-secret", false},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := secure.New(tt.value).IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValue_IsZeroUsesUnderlyingIsZero(t *testing.T) {
	t.Parallel()

	var zero time.Time
	if !secure.New(zero).IsZero() {
		t.Error("IsZero() = false, want true for the zero time.Time")
	}

	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	if secure.New(now).IsZero() {
		t.Error("IsZero() = true, want false for a non-zero time.Time")
	}
}
