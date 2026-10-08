package netclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PublicImageTransport uses the configured route once per request, then pins
// direct and proxy connections to vetted public IPs. It is shared by desktop
// shells; ordinary provider HTTP clients intentionally do not use this policy.
type PublicImageTransport struct {
	ProxyFor       func(*http.Request) (*url.URL, error)
	LookupIP       func(context.Context, string) ([]net.IPAddr, error)
	DialerForProxy func(*url.URL) (StreamDialer, error)
	Options        TransportOptions
}

func NewPublicImageClient(spec ProxySpec, lookup func(context.Context, string) ([]net.IPAddr, error)) (*http.Client, error) {
	proxy, err := ProxyFunc(spec)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: PublicImageTransport{
		ProxyFor: proxy, LookupIP: lookup, DialerForProxy: PublicImageStreamDialer,
		Options: TransportOptions{DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second},
	}}, nil
}

func (rt PublicImageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("public image URL required")
	}
	if _, err := ValidatePublicImageURL(req.URL.String()); err != nil {
		return nil, err
	}
	// Images never inherit model/provider credentials, cookies or the previous
	// image URL (which can contain private query parameters). Clone first.
	req = req.Clone(req.Context())
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	req.Header.Del("Referer")
	req.Header.Del("Proxy-Authorization")
	lookup := rt.LookupIP
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	addresses, err := ResolvePublicImageAddresses(req.Context(), req.URL.Hostname(), lookup)
	if err != nil {
		return nil, err
	}
	var proxyURL *url.URL
	if rt.ProxyFor != nil {
		proxyURL, err = rt.ProxyFor(req)
		if err != nil {
			return nil, err
		}
	}
	proxyURL, err = NormalizePublicImageProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}
	factory := rt.DialerForProxy
	if factory == nil {
		factory = PublicImageStreamDialer
	}
	dialer, err := factory(proxyURL)
	if err != nil {
		return nil, err
	}
	transport, err := NewTransport(ProxySpec{Mode: ModeOff}, rt.Options)
	if err != nil {
		return nil, err
	}
	transport.DisableKeepAlives = true
	transport.MaxResponseHeaderBytes = 64 << 10
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, target := range addresses {
			dialCtx := ctx
			cancel := func() {}
			if rt.Options.DialTimeout > 0 {
				dialCtx, cancel = context.WithTimeout(ctx, rt.Options.DialTimeout)
			}
			conn, err := dialer.DialContext(dialCtx, network, net.JoinHostPort(target.IP.String(), port))
			cancel()
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	resp.Body = &publicImageBody{ReadCloser: resp.Body, closeTransport: transport.CloseIdleConnections}
	return resp, nil
}

type publicImageBody struct {
	io.ReadCloser
	closeTransport func()
}

func (body *publicImageBody) Close() error {
	err := body.ReadCloser.Close()
	body.closeTransport()
	return err
}

func PublicImageStreamDialer(proxy *url.URL) (StreamDialer, error) {
	if proxy == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		return DialerFunc(dialer.DialContext), nil
	}
	return NewStreamDialer(ProxySpec{Mode: ModeCustom, URL: proxy.String()})
}

func NormalizePublicImageProxyURL(proxy *url.URL) (*url.URL, error) {
	if proxy == nil {
		return nil, nil
	}
	copy := *proxy
	copy.Scheme = strings.ToLower(copy.Scheme)
	if copy.Scheme == "" {
		copy.Scheme = "http"
	}
	port, ok := map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[copy.Scheme]
	if !ok || copy.Hostname() == "" {
		return nil, fmt.Errorf("image proxy URL is invalid")
	}
	if copy.Port() == "" {
		copy.Host = net.JoinHostPort(copy.Hostname(), port)
	}
	return &copy, nil
}

func ResolvePublicImageAddresses(ctx context.Context, host string, lookup func(context.Context, string) ([]net.IPAddr, error)) ([]net.IPAddr, error) {
	addresses, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 || len(addresses) > 64 {
		return nil, fmt.Errorf("image host address count is invalid")
	}
	for _, address := range addresses {
		if BlockedPublicImageIP(address.IP) || address.Zone != "" {
			return nil, fmt.Errorf("image host resolved to a non-public address")
		}
	}
	return addresses, nil
}

func ValidatePublicImageURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 16<<10 {
		return "", fmt.Errorf("empty or oversized image URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("image URL must be absolute without credentials")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || BlockedPublicImageHost(u.Hostname()) {
		return "", fmt.Errorf("image URL is not public HTTP(S)")
	}
	u.Fragment = ""
	return u.String(), nil
}

func BlockedPublicImageHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".home.arpa") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return !strings.Contains(host, ".")
	}
	return BlockedPublicImageIP(ip)
}

func BlockedPublicImageIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return true
	}
	// Go's GlobalUnicast classification includes documentation, protocol and
	// translation prefixes. Untrusted images deliberately cannot use those:
	// a public-looking NAT64/6to4 address can encode a private IPv4 target.
	for _, network := range publicImageSpecialNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// This intentionally conservative image-only policy excludes special protocol
// space even where a more specific IANA assignment is globally reachable.
// Provider and ordinary application network clients are unaffected.
// References: IANA IPv4/IPv6 Special-Purpose Address Registries.
var publicImageSpecialNetworks = func() []*net.IPNet {
	var networks []*net.IPNet
	for _, cidr := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64",
		"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16",
	} {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		networks = append(networks, network)
	}
	return networks
}()
