package globalping

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type measurementRequest struct {
	pingOptions *pingOptions
	httpOptions *httpOptions
	Type        measurementType `json:"type"`
	Target      string          `json:"target"`
	Options     interface{}     `json:"measurementOptions,omitempty"`
	Locations   []location      `json:"locations"`
}

func (r *measurementRequest) MarshalJSON() ([]byte, error) {
	if r.Target == "" {
		return nil, fmt.Errorf(".target is empty")
	}
	if len(r.Locations) == 0 {
		return nil, fmt.Errorf(".locations is empty")
	}

	type alias measurementRequest
	a := alias(*r)
	switch r.Type {
	case measurementTypePing:
		if r.pingOptions == nil {
			r.pingOptions = &pingOptions{PacketsCount: 1}
		}
		if r.pingOptions.PacketsCount == 0 {
			r.pingOptions.PacketsCount = 1
		}
		a.Options = r.pingOptions
	case measurementTypeHTTP:
		// TODO: validate
		if r.httpOptions == nil {
			r.httpOptions = &httpOptions{}
		}
		a.Options = r.httpOptions
	default:
		return nil, fmt.Errorf("unknown .type: %s", r.Type)
	}
	return json.Marshal(a)
}

type measurementType string

const (
	measurementTypePing measurementType = "ping"
	measurementTypeHTTP measurementType = "http"
)

type pingOptions struct {
	PacketsCount uint8 `json:"packets,omitempty"`
}

type httpOptions struct {
	Protocol httpProtocol `json:"protocol,omitempty"`
	Request  struct {
		Method  httpMethod        `json:"method,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
		Host    string            `json:"host,omitempty"`
		Path    string            `json:"path,omitempty"`
		Query   string            `json:"query,omitempty"`
	} `json:"request,omitempty"`
	Port uint16 `json:"port,omitempty"`
}

type httpMethod string

const (
	httpMethodHEAD httpMethod = http.MethodHead
)

type httpProtocol string

const (
	httpProtocolHTTP  httpProtocol = "HTTP"
	httpProtocolHTTPS httpProtocol = "HTTPS"
	httpProtocolHTTP2 httpProtocol = "HTTP2"
)

type location struct {
	Region  string `json:"region,omitempty"`
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
	Limit   uint8  `json:"limit"`
}

type responseOnSuccess struct {
	ID          string              `json:"id"`
	Status      string              `json:"status"`
	Results     []measurementResult `json:"results"`
	ProbesCount uint8               `json:"probesCount"`
}

type responseOnError struct {
	Error struct {
		Params  map[string]string `json:"params"` // bad request only
		Type    string            `json:"type"`
		Message string            `json:"message"`
	} `json:"error"`
}

type measurementResult struct {
	Result struct {
		Status          string            `json:"status"`
		HTTPHeaders     map[string]string `json:"headers"` // HTTP measurement only
		ResolvedAddress string            `json:"resolvedAddress"`
		HTTPStatusCode  uint16            `json:"statusCode"` // HTTP measurement only
	} `json:"result"`
	Probe probe `json:"probe"`
}

type probe struct {
	Location location `json:"location"`
}
