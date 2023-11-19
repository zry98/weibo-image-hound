package globalping

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type requestBody struct {
	Type      measurementType `json:"type"`
	Target    string          `json:"target"`
	Locations []location      `json:"locations"`
	//Limit   uint8 `json:"limit,omitempty"`  // global limit not used
	Options interface{} `json:"measurementOptions,omitempty"`
	// options
	PingOptions *pingOptions `json:"-"`
	HTTPOptions *httpOptions `json:"-"`
}

func (r requestBody) MarshalJSON() ([]byte, error) {
	if r.Target == "" {
		return nil, fmt.Errorf(".target is empty")
	}
	if len(r.Locations) == 0 {
		return nil, fmt.Errorf(".locations is empty")
	}

	type alias requestBody
	a := alias(r)
	switch r.Type {
	case measurementTypePing:
		if r.PingOptions != nil {
			if r.PingOptions.Packets == 0 {
				r.PingOptions.Packets = 1
			}
		} else {
			r.PingOptions = &pingOptions{Packets: 1}
		}
		a.Options = r.PingOptions
	case measurementTypeHTTP:
		// TODO: validate
		if r.HTTPOptions != nil {
			a.Options = r.HTTPOptions
		}
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

type location struct {
	Region  string `json:"region,omitempty"`
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
	Magic   string `json:"magic,omitempty"`
	Limit   uint8  `json:"limit"`
}

type pingOptions struct {
	Packets uint8 `json:"packets,omitempty"`
}

type httpOptions struct {
	Request struct {
		Method  httpMethod        `json:"method,omitempty"`
		Host    string            `json:"host,omitempty"`
		Path    string            `json:"path,omitempty"`
		Query   string            `json:"query,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
	} `json:"request,omitempty"`
	Port     uint16       `json:"port,omitempty"`
	Protocol httpProtocol `json:"protocol,omitempty"`
}

type httpMethod string

const (
	httpMethodHEAD httpMethod = http.MethodHead
	//httpMethodGET  httpMethod = http.MethodGet
)

type httpProtocol string

const (
	httpProtocolHTTP  httpProtocol = "HTTP"
	httpProtocolHTTPS httpProtocol = "HTTPS"
	httpProtocolHTTP2 httpProtocol = "HTTP2"
)

type responseBodyOnSuccess struct {
	ID          string              `json:"id"`
	ProbesCount uint8               `json:"probesCount"`
	Status      string              `json:"status"`
	Results     []measurementResult `json:"results"`
}

type responseBodyOnError struct {
	Error struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Params  map[string]string `json:"params"` // only for bad request
	} `json:"error"`
}

type measurementResult struct {
	Probe  probe `json:"probe"`
	Result struct {
		Status          string `json:"status"`
		ResolvedAddress string `json:"resolvedAddress"`
		// HTTP type measurement only
		HTTPStatusCode uint16            `json:"statusCode"`
		HTTPHeaders    map[string]string `json:"headers"`
	} `json:"result"`
}

type probe struct {
	Location location `json:"location"`
}
