package hound

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/andybalholm/brotli"
)

type Result struct {
	Err     error
	Headers http.Header
	IP      net.IP
	Body    []byte
	Status  int
}

func Hunt(ctx context.Context, ch chan<- Result, URL string, port string, IPs []net.IP, headers http.Header) {
	for _, IP := range IPs {
		addr := fmt.Sprintf("%s:%s", IP, port)
		if IP.To4() == nil { // IPv6 address
			addr = fmt.Sprintf("[%s]:%s", IP, port)
		}

		go func(IP net.IP) {
			select {
			case <-ctx.Done():
				return
			default:
				status, respHeaders, body, err := newClient(ctx, addr).
					request(http.MethodGet, URL, headers)
				if err != nil {
					ch <- Result{IP: IP, Err: err}
					return
				}
				ch <- Result{IP: IP, Status: status, Headers: respHeaders, Body: body}
			}
		}(IP)
	}
}

const (
	requestTimeout = 10 * time.Second
	clientTimeout  = 15 * time.Second
)

type client struct {
	*http.Client
	ctx context.Context
}

var (
	baseHeaders = http.Header{
		"Accept":             {"image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"},
		"Accept-Encoding":    {"gzip, deflate, br"},
		"Accept-Language":    {"zh-CN,zh;q=0.9"},
		"Referer":            {"https://weibo.com/"},
		"Sec-Ch-Ua":          {`Google Chrome";v="119", "Chromium";v="119", "Not?A_Brand";v="24`},
		"Sec-Ch-Ua-Mobile":   {"?0"},
		"Sec-Ch-Ua-Platform": {"Windows"},
		"Sec-Fetch-Dest":     {"image"},
		"Sec-Fetch-Mode":     {"no-cors"},
		"Sec-Fetch-Site":     {"cross-site"},
		"User-Agent":         {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"},
	}
	dialer = &net.Dialer{
		Timeout:       clientTimeout,
		FallbackDelay: -1, // disable dual stack
		KeepAlive:     -1,
	}
)

func newClient(ctx context.Context, address string) *client {
	return &client{
		Client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.DialContext(ctx, network, address)
				},
				DisableKeepAlives: true,
				ForceAttemptHTTP2: true,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error { // don't follow 301 redirect
				return http.ErrUseLastResponse
			},
			Timeout: clientTimeout,
		},
		ctx: ctx,
	}
}

func (c *client) request(method string, URL string, reqHeaders http.Header) (statusCode int, respHeaders http.Header, body []byte, err error) {
	ctx, cancel := context.WithTimeout(c.ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, URL, nil)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = baseHeaders.Clone()
	for k, v := range reqHeaders {
		// not using req.Header.Set() and .Del() in case of non-canonical key
		if len(v) > 0 && v[0] != "" {
			req.Header[k] = v
		} else {
			delete(req.Header, k)
		}
	}

	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var respBody []byte
	if resp.Header.Get("content-encoding") == "br" {
		respBody, err = io.ReadAll(brotli.NewReader(resp.Body))
	} else {
		respBody, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return resp.StatusCode, resp.Header, respBody, nil
}
