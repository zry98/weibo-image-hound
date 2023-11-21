package globalping

import (
	"fmt"
	"net"
	"net/http"
)

type Config struct {
	//APIToken string `yaml:"api_token,omitempty"`
}

func NewClient() *client {
	return &client{
		Client: &http.Client{},
		eTags:  make(map[string]string),
	}
}

func (c *client) Resolve(hostname string, locations []string) ([]net.IP, error) {
	if len(locations) == 0 { // use all default regions if none specified
		locations = defaultRegions
	}
	mID, err := c.createMeasurement(hostname, locations)
	if err != nil {
		return nil, fmt.Errorf("failed to create measurement: %w", err)
	}

	mResults, err := c.getMeasurement(mID)
	if err != nil {
		return nil, fmt.Errorf("failed to get measurement: %w", err)
	}

	IPs := make([]net.IP, 0, len(mResults))
	for _, r := range mResults {
		if r.Result.ResolvedAddress != "" {
			IPs = append(IPs, net.ParseIP(r.Result.ResolvedAddress))
		}
	}
	return IPs, nil
}

func (c *client) Probes() ([]string, error) {
	probes, err := c.getProbes()
	if err != nil {
		return nil, fmt.Errorf("failed to get probes: %w", err)
	}

	locations := make([]string, 0, len(probes))
	for _, p := range probes {
		if p.Location.Region != "" {
			locations = append(locations, p.Location.Region)
		}
	}
	return locations, nil
}

func (c *client) Locations() ([]string, error) {
	return defaultRegions, nil
}
