package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVPNGate(t *testing.T) {
	g, err := NewVPNGate([]string{"10.8.0.0/24"}, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	var seen string
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))

	cases := []struct {
		name   string
		remote string
		xff    []string
		path   string
		want   int
		wantIP string
	}{
		{"peer via caddy", "127.0.0.1:5000", []string{"10.8.0.7"}, "/", 200, "10.8.0.7"},
		{"outsider via caddy", "127.0.0.1:5000", []string{"192.168.1.5"}, "/", 404, ""},
		{"spoof left of xff", "127.0.0.1:5000", []string{"10.8.0.7, 203.0.113.9"}, "/", 404, ""},
		{"spoof in earlier header", "127.0.0.1:5000", []string{"10.8.0.7", "203.0.113.9"}, "/", 404, ""},
		{"untrusted direct peer with xff", "172.18.0.5:5000", []string{"10.8.0.7"}, "/", 404, ""},
		{"direct vpn peer", "10.8.0.9:4444", nil, "/api/me", 200, "10.8.0.9"},
		{"loopback no xff", "127.0.0.1:5000", nil, "/", 404, ""},
		{"loopback health", "127.0.0.1:5000", nil, "/api/health", 200, "127.0.0.1"},
		{"garbage xff", "127.0.0.1:5000", []string{"nope"}, "/", 404, ""},
		{"mapped v6 peer", "[::ffff:10.8.0.3]:1", nil, "/", 200, "10.8.0.3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen = ""
			r := httptest.NewRequest("GET", c.path, nil)
			r.RemoteAddr = c.remote
			for _, v := range c.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status %d, want %d", w.Code, c.want)
			}
			if seen != c.wantIP {
				t.Fatalf("RemoteAddr %q, want %q", seen, c.wantIP)
			}
		})
	}
}

func TestNewVPNGateRequiresSubnet(t *testing.T) {
	if _, err := NewVPNGate(nil, nil); err == nil {
		t.Fatal("expected error for empty vpn_cidrs")
	}
	if _, err := NewVPNGate([]string{"bogus"}, nil); err == nil {
		t.Fatal("expected error for bad cidr")
	}
}
