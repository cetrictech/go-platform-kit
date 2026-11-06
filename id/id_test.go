package id

import "testing"

func TestUUID(t *testing.T) {
	for _, f := range []func() (UUID, error){NewV4, NewV7} {
		u, e := f()
		if e != nil || Validate(u.String()) != nil {
			t.Fatal(u, e)
		}
		v, e := Parse(u.String())
		if e != nil || u != v {
			t.Fatal(e)
		}
	}
}
