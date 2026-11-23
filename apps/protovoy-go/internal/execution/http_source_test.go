package execution

import "testing"

func TestEncodeHTTPSource(t *testing.T) {
	for _, test := range []struct{ kind, source, want string }{
		{"graphql", `{"type":"graphql","query":"query Test { health }","variables":{"n":9007199254740993},"operationName":"Test"}`, `{"query":"query Test { health }","variables":{"n":9007199254740993},"operationName":"Test"}`},
		{"jsonrpc", `{"type":"jsonrpc","method":"echo","params":[2],"id":null}`, `{"jsonrpc":"2.0","method":"echo","params":[2],"id":null}`},
		{"messagepack", `{"a":2}`, "gaFhAg=="},
		{"messagepack", `9007199254740993`, "zwAgAAAAAAAB"},
	} {
		if got, err := EncodeHTTPSource(test.kind, test.source); err != nil || got != test.want {
			t.Errorf("encode %s: got %q (%v), want %q", test.kind, got, err, test.want)
		}
	}
	for _, test := range []struct{ kind, source string }{
		{"graphql", `{"type":"graphql","query":""}`},
		{"jsonrpc", `{"type":"graphql","method":"echo"}`},
		{"messagepack", `18446744073709551616`},
		{"messagepack", `null true`},
	} {
		if _, err := EncodeHTTPSource(test.kind, test.source); err == nil {
			t.Errorf("accepted invalid %s source", test.kind)
		}
	}
}
