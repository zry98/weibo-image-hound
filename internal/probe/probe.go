package probe

import "net"

type Provider interface {
	// Resolve returns the resolved IP addresses of the given hostname from the given locations.
	Resolve(hostname string, locations []string) ([]net.IP, error)
	// Locations returns all currently supported locations of the provider.
	Locations() ([]string, error)
}
