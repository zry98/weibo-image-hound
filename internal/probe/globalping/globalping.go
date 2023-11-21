package globalping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

const (
	baseURL                      = "https://api.globalping.io/v1"
	requestTimeout               = 15 * time.Second
	getMeasurementInterval       = 5 * time.Second
	getMeasurementOverallTimeout = 1 * time.Minute
)

var (
	baseReqHeaders = http.Header{
		"content-type":    {"application/json"},
		"accept":          {"application/json"},
		"accept-encoding": {"br, gzip, deflate"},
		"user-agent":      {"WeiboImageHound/1.0 (https://github.com/zry98/weibo-image-hound)"},
	}

	// Geographic Region names based on the UN [Standard Country or Area Codes for Statistical Use (M49)](https://unstats.un.org/unsd/methodology/m49/).
	defaultRegions = []string{"Northern Africa", "Eastern Africa", "Middle Africa", "Southern Africa", "Western Africa", "Caribbean", "Central America", "South America", "Northern America", "Central Asia", "Eastern Asia", "South-eastern Asia", "Southern Asia", "Western Asia", "Eastern Europe", "Northern Europe", "Southern Europe", "Western Europe", "Australia and New Zealand", "Melanesia", "Micronesia", "Polynesia"}
)

// client represents a client for the GlobalPing API.
type client struct {
	*http.Client
	eTags map[string]string
	mu    sync.Mutex
}

// createMeasurement creates a new measurement and returns its ID.
// API `POST /v1/measurements`, documentation at https://www.jsdelivr.com/docs/api.globalping.io#post-/v1/measurements
func (c *client) createMeasurement(hostname string, regions []string) (string, error) {
	if hostname == "" {
		return "", fmt.Errorf("no hostname specified")
	}
	if len(regions) == 0 {
		return "", fmt.Errorf("no regions specified")
	}
	mLocations := make([]location, 0, len(regions))
	for _, r := range regions {
		mLocations = append(mLocations, location{
			Region: r,
			Limit:  5,
		})
	}

	reqBody, err := json.Marshal(measurementRequest{
		Type:      measurementTypePing,
		Target:    hostname,
		Locations: mLocations,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	URL := baseURL + "/measurements"
	body, err := c.request(http.MethodPost, URL, bytes.NewBuffer(reqBody), nil)
	if err != nil {
		return "", err
	}

	var r responseOnSuccess
	if err = json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	if r.ProbesCount == 0 {
		return "", fmt.Errorf("no probes available")
	}
	if r.ID == "" {
		return "", fmt.Errorf("invalid response: %s", string(body))
	}
	return r.ID, nil
}

// getMeasurement returns the results of the measurement with the given ID.
// API `GET /v1/measurements/{id}`, documentation at https://www.jsdelivr.com/docs/api.globalping.io#get-/v1/measurements/-id-
func (c *client) getMeasurement(ID string) ([]measurementResult, error) {
	if ID == "" {
		return nil, fmt.Errorf("no measurement ID specified")
	}
	URL := baseURL + "/measurements/" + ID
	defer func() {
		c.mu.Lock()
		delete(c.eTags, URL)
		c.mu.Unlock()
	}()

	ticker := time.NewTicker(getMeasurementInterval)
	defer ticker.Stop()
	overallTimeout := time.NewTimer(getMeasurementOverallTimeout)
	defer overallTimeout.Stop()
	for {
		select {
		case <-ticker.C:
			body, err := c.request(http.MethodGet, URL, nil, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get measurement: %v\n", err)
				continue
			}
			if body == nil { // HTTP 304 Not Modified
				fmt.Fprintf(os.Stderr, "Measurement %s in progress...\n", ID)
				continue
			}

			var r responseOnSuccess
			if err = json.Unmarshal(body, &r); err != nil {
				return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
			}
			if r.ID == "" {
				return nil, fmt.Errorf("invalid response: %s", string(body))
			}
			switch r.Status {
			case "in-progress":
				fmt.Fprintf(os.Stderr, "Measurement %s in progress...\n", r.ID)
				continue
			case "finished":
				fmt.Fprintf(os.Stderr, "Measurement %s finished with %d results.\n", r.ID, len(r.Results))
				return r.Results, nil
			default:
				return nil, fmt.Errorf("invalid response: unknown status \"%s\"", r.Status)
			}
		case <-overallTimeout.C:
			return nil, fmt.Errorf("timeout")
		}
	}
}

// getProbes returns a list of all currently connected probes.
// API `GET /v1/probes`, documentation at https://www.jsdelivr.com/docs/api.globalping.io#get-/v1/probes
func (c *client) getProbes() ([]probe, error) {
	URL := baseURL + "/probes"
	body, err := c.request(http.MethodGet, URL, nil, nil)
	if err != nil {
		return nil, err
	}

	var probes []probe
	if err = json.Unmarshal(body, &probes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return probes, nil
}

// request sends a request to the API and returns the response body.
func (c *client) request(method string, URL string, reqBody io.Reader, reqHeaders http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, URL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = baseReqHeaders.Clone()
	for k, v := range reqHeaders {
		// not using req.Header.Set() and .Del() in case of non-canonical key
		if len(v) > 0 && v[0] != "" {
			req.Header[k] = v
		} else {
			delete(req.Header, k)
		}
	}
	if method == http.MethodGet {
		req.Header.Del("content-type")
		c.mu.Lock()
		if eTag, ok := c.eTags[URL]; ok && eTag != "" {
			req.Header.Set("if-none-match", eTag)
		}
		c.mu.Unlock()
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var body []byte
	if resp.Header.Get("content-encoding") == "br" {
		body, err = io.ReadAll(brotli.NewReader(resp.Body))
	} else {
		body, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	eTag := resp.Header.Get("ETag")
	if method == http.MethodGet && eTag != "" {
		c.mu.Lock()
		c.eTags[URL] = eTag
		c.mu.Unlock()
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted:
		return body, nil
	case http.StatusNotModified:
		return nil, nil
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		var r responseOnError
		if err = json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("API returned error: (type \"%s\") %s", r.Error.Type, r.Error.Message))
		if resp.StatusCode == http.StatusBadRequest && len(r.Error.Params) > 0 {
			sb.WriteString("\nError params:\n")
			for p, msg := range r.Error.Params {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", p, msg))
			}
		}
		return nil, fmt.Errorf(sb.String())
	case http.StatusTooManyRequests:
		if ttr, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return nil, fmt.Errorf("too many requests, try again in %s", (time.Duration(ttr) * time.Second).String())
		}
		return nil, fmt.Errorf("too many requests")
	}
	return body, fmt.Errorf("unexpected response (HTTP %d)", resp.StatusCode)
}
