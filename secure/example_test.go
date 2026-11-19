package secure_test

import (
	"encoding/json"
	"fmt"

	"github.com/tenntenn/exp/secure"
	"github.com/tenntenn/exp/secure/unsecure"
)

func ExampleValue() {
	v := secure.New("sample-secret")

	// String, the fmt verbs and logs show the mask, not the value.
	fmt.Println(v)

	// Unwrap through unsecure when you mean to show the value.
	fmt.Println(unsecure.Value(v))
	// Output:
	// ****
	// sample-secret
}

func ExampleNew_withMask() {
	v := secure.New("sample-secret", secure.WithMask(secure.MaskDefault[string]('x', 6)))
	fmt.Println(v)
	// Output:
	// xxxxxx
}

func ExampleValue_json() {
	// MarshalJSON writes the underlying value so it can be read back; only the
	// human-facing forms are masked.
	v := secure.New("sample-secret")

	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	// Output:
	// "sample-secret"
}
