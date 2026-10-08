package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
)

const (
	remoteMarkdownImagePath     = "/__reasonix_remote_markdown_image"
	remoteMarkdownImageMaxBytes = 10 * 1024 * 1024
	remoteMarkdownImageTimeout  = 20 * time.Second
)

type remoteMarkdownImageClientFactory func(netclient.ProxySpec) (*http.Client, error)

type remoteMarkdownImageLookupIP func(context.Context, string) ([]net.IPAddr, error)

type remoteMarkdownImageDialerFactory func(*url.URL) (netclient.StreamDialer, error)

func newRemoteMarkdownImageClient(spec netclient.ProxySpec) (*http.Client, error) {
	return newRemoteMarkdownImageClientWithLookup(spec, net.DefaultResolver.LookupIPAddr)
}

func newRemoteMarkdownImageClientWithLookup(spec netclient.ProxySpec, lookupIP remoteMarkdownImageLookupIP) (*http.Client, error) {
	return netclient.NewPublicImageClient(spec, lookupIP)
}

type remoteMarkdownImageRoundTripper struct {
	proxyFor       func(*http.Request) (*url.URL, error)
	lookupIP       remoteMarkdownImageLookupIP
	dialerForProxy remoteMarkdownImageDialerFactory
	options        netclient.TransportOptions
}

func (rt remoteMarkdownImageRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return (netclient.PublicImageTransport{
		ProxyFor: rt.proxyFor, LookupIP: rt.lookupIP,
		DialerForProxy: rt.dialerForProxy, Options: rt.options,
	}).RoundTrip(req)
}

func newRemoteMarkdownImageStreamDialer(proxyURL *url.URL) (netclient.StreamDialer, error) {
	return netclient.PublicImageStreamDialer(proxyURL)
}

func normalizedRemoteMarkdownImageProxyURL(proxyURL *url.URL) (*url.URL, error) {
	return netclient.NormalizePublicImageProxyURL(proxyURL)
}

func resolveRemoteMarkdownImageAddresses(ctx context.Context, host string, lookupIP remoteMarkdownImageLookupIP) ([]net.IPAddr, error) {
	return netclient.ResolvePublicImageAddresses(ctx, host, lookupIP)
}

// remoteMarkdownImageMiddleware keeps external images out of the WebView2
// network stack. The backend fetches them with Reasonix's proxy configuration,
// validates the response, sanitizes SVG, and serves only bounded image bytes
// from the local Wails origin.
func (a *App) remoteMarkdownImageMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != remoteMarkdownImagePath {
				next.ServeHTTP(w, r)
				return
			}
			cfg, err := config.Load()
			if err != nil {
				http.Error(w, "remote image unavailable", http.StatusBadGateway)
				return
			}
			serveRemoteMarkdownImage(w, r, cfg.NetworkProxySpec(), newRemoteMarkdownImageClient)
		})
	}
}

func serveRemoteMarkdownImage(
	w http.ResponseWriter,
	r *http.Request,
	spec netclient.ProxySpec,
	clientFactory remoteMarkdownImageClientFactory,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawURL, err := validateRemoteMarkdownImageURL(r.URL.Query().Get("url"))
	if err != nil {
		http.Error(w, "invalid remote image URL", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), remoteMarkdownImageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		http.Error(w, "invalid remote image URL", http.StatusBadRequest)
		return
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/bmp,image/svg+xml;q=0.9,*/*;q=0.1")
	req.Header.Set("User-Agent", "Reasonix-Desktop/1.0")

	client, err := clientFactory(spec)
	if err != nil {
		http.Error(w, "remote image proxy configuration is invalid", http.StatusBadGateway)
		return
	}
	clientCopy := *client
	client = &clientCopy
	client.Timeout = remoteMarkdownImageTimeout
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if _, err := validateRemoteMarkdownImageURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	}

	// The production transport resolves every initial and redirected target to
	// public IPs and pins direct/proxied dials to those vetted addresses.
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "remote image fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		http.Error(w, "remote image fetch failed", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteMarkdownImageMaxBytes+1))
	if err != nil || len(body) == 0 || len(body) > remoteMarkdownImageMaxBytes {
		http.Error(w, "remote image response is invalid", http.StatusBadGateway)
		return
	}
	body, mimeType := safeRemoteMarkdownImage(body)
	if mimeType == "" {
		http.Error(w, "remote response is not a supported image", http.StatusUnsupportedMediaType)
		return
	}
	if err := validateMarkdownImageBytes(body, mimeType); err != nil {
		if errors.Is(err, errMarkdownImageTooLarge) {
			http.Error(w, "remote image exceeds the decode budget", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "remote response is not a valid image", http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func validateRemoteMarkdownImageURL(raw string) (string, error) {
	return netclient.ValidatePublicImageURL(raw)
}

func blockedRemoteMarkdownImageHost(host string) bool {
	return netclient.BlockedPublicImageHost(host)
}

func blockedRemoteMarkdownImageIP(ip net.IP) bool {
	return netclient.BlockedPublicImageIP(ip)
}

func safeRemoteMarkdownImage(body []byte) ([]byte, string) {
	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(http.DetectContentType(head), ";", 2)[0])) {
	case "image/png":
		return body, "image/png"
	case "image/jpeg":
		return body, "image/jpeg"
	case "image/gif":
		return body, "image/gif"
	case "image/webp":
		return body, "image/webp"
	case "image/bmp":
		return body, "image/bmp"
	case "image/x-icon":
		return body, "image/x-icon"
	}
	if sanitized, ok := sanitizeRemoteMarkdownSVG(body); ok {
		return sanitized, "image/svg+xml"
	}
	return nil, ""
}

var remoteMarkdownSVGForbiddenElements = map[string]bool{
	"animate":          true,
	"animatemotion":    true,
	"animatetransform": true,
	"audio":            true,
	"embed":            true,
	"foreignobject":    true,
	"iframe":           true,
	"object":           true,
	"script":           true,
	"set":              true,
	"style":            true,
	"video":            true,
}

func sanitizeRemoteMarkdownSVG(body []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	trimmed = bytes.TrimPrefix(trimmed, []byte{0xef, 0xbb, 0xbf})
	trimmed = bytes.TrimSpace(trimmed)
	if len(trimmed) == 0 {
		return nil, false
	}

	decoder := xml.NewDecoder(bytes.NewReader(trimmed))
	decoder.Strict = true
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	rootSeen := false
	rootDepth := 0
	skipDepth := 0

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false
		}
		switch value := token.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			name := strings.ToLower(value.Name.Local)
			if !rootSeen {
				if name != "svg" || (value.Name.Space != "" && value.Name.Space != "http://www.w3.org/2000/svg") {
					return nil, false
				}
				rootSeen = true
			} else if rootDepth == 0 {
				return nil, false
			}
			if remoteMarkdownSVGForbiddenElements[name] {
				skipDepth = 1
				continue
			}
			attrs := value.Attr[:0]
			for _, attr := range value.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				if strings.HasPrefix(attrName, "on") || attrName == "srcset" ||
					(attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && attrName == "base") {
					continue
				}
				if attrName == "href" || attrName == "src" {
					if !safeRemoteMarkdownSVGReference(attr.Value) {
						continue
					}
				} else if !safeRemoteMarkdownSVGAttributeValue(attr.Value) {
					continue
				}
				attrs = append(attrs, attr)
			}
			value.Attr = attrs
			if err := encoder.EncodeToken(value); err != nil {
				return nil, false
			}
			rootDepth++
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if rootDepth <= 0 {
				return nil, false
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, false
			}
			rootDepth--
		case xml.CharData:
			if skipDepth == 0 && (!rootSeen || rootDepth == 0) {
				if len(bytes.TrimSpace(value)) != 0 {
					return nil, false
				}
				continue
			}
			if skipDepth == 0 {
				if err := encoder.EncodeToken(value); err != nil {
					return nil, false
				}
			}
		case xml.Comment:
			// Comments are not needed for display and can hide suspicious payloads.
		case xml.Directive, xml.ProcInst:
			// Drop DTDs and processing instructions; SVG does not need them here.
		default:
			if skipDepth == 0 {
				if err := encoder.EncodeToken(value); err != nil {
					return nil, false
				}
			}
		}
	}
	if !rootSeen || rootDepth != 0 || skipDepth != 0 || encoder.Flush() != nil {
		return nil, false
	}
	return out.Bytes(), true
}

func safeRemoteMarkdownSVGReference(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(value, "#") {
		return true
	}
	for _, prefix := range []string{
		"data:image/png;base64,",
		"data:image/jpeg;base64,",
		"data:image/gif;base64,",
		"data:image/webp;base64,",
		"data:image/bmp;base64,",
		"data:image/x-icon;base64,",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func safeRemoteMarkdownSVGAttributeValue(raw string) bool {
	value := strings.ToLower(raw)
	if strings.Contains(value, "javascript:") || strings.Contains(value, "vbscript:") || strings.Contains(value, "data:text/html") {
		return false
	}
	for {
		index := strings.Index(value, "url(")
		if index < 0 {
			return !strings.Contains(value, "@import") && !strings.Contains(value, "expression(")
		}
		value = value[index+4:]
		end := strings.IndexByte(value, ')')
		if end < 0 {
			return false
		}
		target := strings.Trim(strings.TrimSpace(value[:end]), "\"'")
		if !strings.HasPrefix(target, "#") {
			return false
		}
		value = value[end+1:]
	}
}
