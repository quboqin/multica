package deviceruntime

import "testing"

func TestReversePortForWebURL(t *testing.T) {
	for _, tc := range []struct {
		url  string
		port string
		ok   bool
	}{
		{url: "http://127.0.0.1:13000/login", port: "13000", ok: true},
		{url: "http://localhost", port: "80", ok: true},
		{url: "https://[::1]", port: "443", ok: true},
		{url: "http://10.0.2.2:13000", ok: false},
		{url: "https://preview.example.test", ok: false},
	} {
		port, ok := reversePortForWebURL(tc.url)
		if port != tc.port || ok != tc.ok {
			t.Errorf("reversePortForWebURL(%q) = (%q, %v), want (%q, %v)", tc.url, port, ok, tc.port, tc.ok)
		}
	}
}
