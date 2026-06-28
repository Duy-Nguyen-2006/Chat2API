// Package httpclient provides a TLS-fingerprint-impersonating HTTP client
// backed by bogdanfinn/tls-client. It exposes a minimal Doer interface over
// the standard net/http types, hiding the underlying fhttp fork so the rest
// of the codebase stays decoupled and easy to mock in tests.
//
// The default profile impersonates a recent Chrome ClientHello + HTTP/2
// fingerprint, which is required to reach chatgpt.com through Cloudflare.
package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// Doer is the minimal HTTP client contract used throughout the codebase.
// It mirrors net/http's *http.Client.Do so any implementation returning a
// standard *http.Response (stdlib or our tls-client adapter) satisfies it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultProfile is the browser profile used when callers don't override it.
// Chrome 133 balances recency (Cloudflare expectations) with upstream maturity.
var DefaultProfile = profiles.Chrome_133

// Options configures New.
type Options struct {
	// Profile is the TLS/HTTP2 fingerprint profile. Required — pass
	// DefaultProfile (or DefaultOptions()) when you don't need a specific one.
	Profile profiles.ClientProfile
	// ProxyURL is an optional proxy: http://, https://, socks5://, socks5h://.
	ProxyURL string
	// TimeoutSeconds is the per-request timeout. Use 0 (or negative) to
	// disable — REQUIRED for long-lived SSE streams where the body must stay
	// open; apply deadlines via context instead.
	TimeoutSeconds int
	// InsecureSkipVerify disables TLS certificate verification. Use only with
	// a self-hosted proxy that performs MITM.
	InsecureSkipVerify bool
}

// DefaultOptions returns Options pre-populated with DefaultProfile and an
// infinite (SSE-safe) timeout. Mutate the returned struct to override fields.
func DefaultOptions() Options {
	return Options{Profile: DefaultProfile, TimeoutSeconds: 0}
}

// Client adapts a tls-client (fhttp-based) HttpClient to the net/http Doer
// interface by translating requests and responses in both directions.
type Client struct {
	raw tls_client.HttpClient
}

// New builds a TLS-impersonating Client wrapping bogdanfinn/tls-client.
// Pass Options{TimeoutSeconds: 0} for streaming endpoints.
func New(opts Options) (*Client, error) {
	clientOpts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(opts.Profile),
		tls_client.WithNotFollowRedirects(),
	}
	if opts.TimeoutSeconds > 0 {
		clientOpts = append(clientOpts, tls_client.WithTimeoutSeconds(opts.TimeoutSeconds))
	}
	if opts.ProxyURL != "" {
		clientOpts = append(clientOpts, tls_client.WithProxyUrl(opts.ProxyURL))
	}
	if opts.InsecureSkipVerify {
		clientOpts = append(clientOpts, tls_client.WithInsecureSkipVerify())
	}

	raw, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("httpclient: build tls-client: %w", err)
	}
	return &Client{raw: raw}, nil
}

// MustNew is like New but panics on error. Use only for package-level
// defaults where a failure is a programmer error.
func MustNew(opts Options) *Client {
	c, err := New(opts)
	if err != nil {
		panic(err)
	}
	return c
}

// Do translates a net/http request into the fhttp fork, dispatches it through
// the underlying tls-client, and translates the response back. The returned
// *http.Response.Body must be closed by the caller.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	freq, err := toFhttpRequest(req)
	if err != nil {
		return nil, err
	}
	fresp, err := c.raw.Do(freq)
	if err != nil {
		return nil, err
	}
	return fromFhttpResponse(fresp), nil
}

// toFhttpRequest converts a *net/http.Request into a *fhttp.Request,
// preserving method, URL, headers, and body.
func toFhttpRequest(req *http.Request) (*fhttp.Request, error) {
	var body io.Reader
	if req.Body != nil {
		body = req.Body
	}
	freq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("httpclient: build fhttp request: %w", err)
	}
	// Copy headers. Header ordering matters for fingerprinting; fhttp.Request
	// preserves insertion order, so add in the source's canonical order.
	for key := range req.Header {
		for _, val := range req.Header[key] {
			freq.Header.Add(key, val)
		}
	}
	if req.Host != "" {
		freq.Host = req.Host
	}
	// Ensure Host header is set when URL omits it but Host field is populated.
	if freq.Header.Get("Host") == "" && req.Host != "" {
		freq.Header.Set("Host", req.Host)
	}
	return freq, nil
}

// fromFhttpResponse converts a *fhttp.Response into a *net/http.Response.
// The body reader is shared (closing the returned Response.Body closes the
// underlying fhttp body).
func fromFhttpResponse(fresp *fhttp.Response) *http.Response {
	resp := &http.Response{
		Status:     fresp.Status,
		StatusCode: fresp.StatusCode,
		Proto:      fresp.Proto,
		ProtoMajor: fresp.ProtoMajor,
		ProtoMinor: fresp.ProtoMinor,
		Body:       fresp.Body,
		Close:      fresp.Close,
		ContentLength: fresp.ContentLength,
		Trailer:    http.Header(fresp.Trailer),
		Request:    nil,
	}
	// Header is map[string][]string in both; copy by value to detach.
	hdr := make(http.Header, len(fresp.Header))
	for k, vs := range fresp.Header {
		// fhttp may store header keys with original casing; canonicalize so
		// downstream code using http.Header.Get works as expected.
		canonical := http.CanonicalHeaderKey(k)
		for _, v := range vs {
			hdr.Add(canonical, v)
		}
	}
	resp.Header = hdr
	resp.Uncompressed = fresp.Uncompressed
	return resp
}

// SetProxy updates the proxy at runtime. Empty clears it.
func (c *Client) SetProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	return c.raw.SetProxy(proxyURL)
}

// parseProxyScheme returns true for proxy URLs the client accepts.
func parseProxyScheme(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "socks5://") || strings.HasPrefix(u, "socks5h://")
}
