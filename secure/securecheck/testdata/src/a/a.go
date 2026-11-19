package a

import (
	"github.com/tenntenn/exp/secure"
	"github.com/tenntenn/exp/secure/unsecure"
)

func useSecure() {
	v := secure.New("sample-secret")
	_ = v.Raw()           // want `secure\.Value\.Raw exposes the underlying secret value`
	_ = v.String()        // ok: String returns the mask
	_ = unsecure.Value(v) // ok: allowed unwrap through unsecure
}

func useMethodExpr() {
	v := secure.New("sample-secret")
	_ = secure.Value[string].Raw(v) // want `secure\.Value\.Raw exposes the underlying secret value`
}

func useMethodValue() {
	v := secure.New("sample-secret")
	f := v.Raw
	_ = f() // want `secure\.Value\.Raw exposes the underlying secret value`
}

// other has its own Raw method that must not be reported.
type other struct{}

func (other) Raw() string { return "" }

func useOther() {
	var o other
	_ = o.Raw() // ok: not secure.Value
}
