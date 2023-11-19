package globalping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

const (
	baseURL                      = "https://api.globalping.io/v1"
	requestTimeout               = 10 * time.Second
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
	validRegions = []string{"Northern Africa", "Eastern Africa", "Middle Africa", "Southern Africa", "Western Africa", "Caribbean", "Central America", "South America", "Northern America", "Central Asia", "Eastern Asia", "South-eastern Asia", "Southern Asia", "Western Asia", "Eastern Europe", "Northern Europe", "Southern Europe", "Western Europe", "Australia and New Zealand", "Melanesia", "Micronesia", "Polynesia"}
)

// client represents a client for the GlobalPing API.
type client struct {
	*http.Client
	//token string
	eTag string
}

// createMeasurement creates a new measurement and returns its ID.
// API `POST /v1/measurements`, documentation at https://www.jsdelivr.com/docs/api.globalping.io#post-/v1/measurements
func (c *client) createMeasurement(hostname string, regions []string) (ID string, err error) {
	mLocations := make([]location, 0, len(regions))
	for _, r := range regions {
		if slices.Contains(validRegions, r) {
			mLocations = append(mLocations, location{
				Magic: r, // FIXME: wait for API to fix Region filter
				Limit: 5,
			})
		}
	}
	if len(mLocations) == 0 {
		return "", fmt.Errorf("no valid region specified")
	}

	reqBody, err := json.Marshal(requestBody{
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

	var r responseBodyOnSuccess
	if err = json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	if r.ProbesCount == 0 {
		return "", fmt.Errorf("no probes available")
	}
	if r.ID == "" {
		return "", fmt.Errorf("no ID returned")
	}
	return r.ID, nil
}

// getMeasurement returns the results of the measurement with the given ID.
// API `GET /v1/measurements/{id}`, documentation at https://www.jsdelivr.com/docs/api.globalping.io#get-/v1/measurements/-id-
func (c *client) getMeasurement(ID string) ([]measurementResult, error) {
	if ID == "" {
		return nil, fmt.Errorf("invalid measurement ID")
	}
	URL := baseURL + "/measurements/" + ID
	defer func() { c.eTag = "" }()

	ticker := time.NewTicker(getMeasurementInterval)
	defer ticker.Stop()
	overallTimeout := time.NewTimer(getMeasurementOverallTimeout)
	defer overallTimeout.Stop()
	for {
		select {
		case <-ticker.C:
			body, err := c.request(http.MethodGet, URL, nil, nil)
			if err != nil {
				fmt.Printf("failed to get measurement: %v\n", err)
				continue
			}
			if body == nil { // HTTP 304 Not Modified
				continue
			}

			var r responseBodyOnSuccess
			if err = json.Unmarshal(body, &r); err != nil {
				return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
			}
			if r.ID == "" {
				return nil, fmt.Errorf("invalid response")
			}
			switch r.Status {
			case "in-progress":
				fmt.Printf("Measurement %s in progress...\n", r.ID)
				continue
			case "finished":
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
	body, err := c.request(http.MethodGet, baseURL+"/probes", nil, nil)
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
		if v != nil && len(v) > 0 && v[0] != "" {
			req.Header[k] = v
		} else {
			delete(req.Header, k)
		}
	}
	if method != http.MethodPost {
		req.Header.Del("content-type")
	}
	if c.eTag != "" {
		req.Header.Set("if-none-match", c.eTag)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var respBody []byte
	if resp.Header.Get("content-encoding") == "br" {
		respBody, err = io.ReadAll(brotli.NewReader(resp.Body))
	} else {
		respBody, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	eTag := resp.Header.Get("ETag")
	if eTag != "" {
		c.eTag = eTag
	}
	switch resp.StatusCode {
	case http.StatusOK:
		fallthrough
	case http.StatusAccepted:
		return respBody, nil
	case http.StatusNotModified:
		return nil, nil
	case http.StatusBadRequest:
		fallthrough
	case http.StatusNotFound:
		fallthrough
	case http.StatusUnprocessableEntity:
		var body responseBodyOnError
		if err = json.Unmarshal(respBody, &body); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("API returned error: (type \"%s\") %s", body.Error.Type, body.Error.Message))
		if resp.StatusCode == http.StatusBadRequest && len(body.Error.Params) > 0 {
			sb.WriteString("\nError params:")
			for _, p := range body.Error.Params {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", p, body.Error.Params[p]))
			}
		}
		return nil, fmt.Errorf(sb.String())
	case http.StatusTooManyRequests:
		if ttr, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return nil, fmt.Errorf("too many requests, try again in %s", (time.Duration(ttr) * time.Second).String())
		}
		return nil, fmt.Errorf("too many requests")
	}
	return respBody, fmt.Errorf("unexpected response (HTTP %d)", resp.StatusCode)
}
