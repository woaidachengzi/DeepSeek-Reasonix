package netclient

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPublicImageTransportPinsDialStripsHeadersAndKeepsCallerUnchanged(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "images.example.com" {
			t.Error("original HTTP host changed")
		}
		for _, name := range []string{"Authorization", "Cookie", "Referer", "Proxy-Authorization"} {
			if r.Header.Get(name) != "" {
				t.Error("private image header reached target", name)
			}
		}
		_, _ = w.Write([]byte("owned pixels"))
	}))
	defer target.Close()
	dialed, routes := "", 0
	transport := PublicImageTransport{
		ProxyFor: func(*http.Request) (*url.URL, error) { routes++; return nil, nil },
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		DialerForProxy: func(*url.URL) (StreamDialer, error) {
			return DialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
				dialed = address
				return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(target.URL, "http://"))
			}), nil
		},
	}
	request, _ := http.NewRequest(http.MethodGet, "http://images.example.com/image", nil)
	for _, name := range []string{"Authorization", "Cookie", "Referer", "Proxy-Authorization"} {
		request.Header.Set(name, "private-owned-fixture")
	}
	reply, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer reply.Body.Close()
	data, err := io.ReadAll(reply.Body)
	if err != nil || string(data) != "owned pixels" || dialed != "93.184.216.34:80" || routes != 1 {
		t.Fatal("pinned route failed")
	}
	if request.Header.Get("Authorization") != "private-owned-fixture" {
		t.Fatal("transport mutated caller's request")
	}
}

func TestPublicImageTransportRejectsEveryNonPublicDNSAnswerBeforeDial(t *testing.T) {
	for _, addresses := range [][]net.IPAddr{
		{{IP: net.ParseIP("127.0.0.1")}},
		{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.1")}},
		{{IP: net.ParseIP("100.100.100.200")}},
		{{IP: net.ParseIP("::ffff:169.254.169.254")}},
		{{IP: net.ParseIP("64:ff9b::a9fe:a9fe")}},
		{{IP: net.ParseIP("2002:a9fe:a9fe::1")}},
		{},
	} {
		transport := PublicImageTransport{
			LookupIP:       func(context.Context, string) ([]net.IPAddr, error) { return addresses, nil },
			DialerForProxy: func(*url.URL) (StreamDialer, error) { t.Fatal("unsafe DNS reached dialer"); return nil, nil },
		}
		request, _ := http.NewRequest(http.MethodGet, "http://images.example.com/image", nil)
		if _, err := transport.RoundTrip(request); err == nil {
			t.Fatal("unsafe DNS accepted")
		}
	}
}

func TestPublicImageSpecialAddressPolicy(t *testing.T) {
	for _, raw := range []string{
		"0.1.2.3", "100.100.100.200", "192.0.0.1", "192.0.2.1", "192.88.99.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "240.0.0.1",
		"::169.254.169.254", "::ffff:10.0.0.1", "64:ff9b::a9fe:a9fe", "64:ff9b:1::1",
		"100::1", "100:0:0:1::1", "2001::1", "2001:2::1", "2001:db8::1",
		"2002:a9fe:a9fe::1", "3fff::1", "5f00::1", "fe80::1", "fc00::1",
	} {
		t.Run(raw, func(t *testing.T) {
			if !BlockedPublicImageIP(net.ParseIP(raw)) {
				t.Fatal("special image target accepted")
			}
		})
	}
	for _, raw := range []string{"93.184.216.34", "8.8.8.8", "::ffff:8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if BlockedPublicImageIP(net.ParseIP(raw)) {
			t.Fatal("ordinary public address rejected", raw)
		}
	}
}
