package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"

	"weibo-image-hound/internal/probe"
	"weibo-image-hound/internal/probe/globalping"
	"weibo-image-hound/internal/weibo"
)

// cacheCmd represents the cache command
var cacheCmd = &cobra.Command{
	Use:   "cache [flags]",
	Short: "Cache resolved IP addresses for all Weibo image hostnames",
	Long: `Cache resolved IP addresses for all Weibo image hostnames. 
Example: weibo-image-hound cache -p globalping`,
	Run: cache,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.Flags().StringP("provider", "p", "globalping", "Probe provider to use.")
	cacheCmd.Flags().BoolP("force", "f", false, "Force overwrite existing cached resolves.")
}

func cache(cmd *cobra.Command, args []string) {
	var provider probe.Provider
	name := cmd.Flag("provider").Value.String()
	switch name {
	case "globalping":
		provider = globalping.NewClient()
	default:
		panic(fmt.Errorf("unknown provider: %s", name))
	}

	// cache locations
	if config.Cache.Locations == nil {
		config.Cache.Locations = make(map[string][]string)
	}
	locations, err := provider.Locations()
	if err != nil {
		panic(fmt.Errorf("failed to get locations: %w", err))
	}
	locations = unique(locations)
	config.Cache.Locations[name] = locations
	saveConfig()
	fmt.Printf("Cached %d locations for probe provider \"%s\".\n", len(locations), name)

	// cache resolves
	resolves := config.Cache.Resolves
	if cmd.Flag("force").Changed { // force overwrite
		resolves = nil
	}
	for _, hostname := range weibo.Hostnames() {
		IPs, err := provider.Resolve(hostname, locations)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve %s: %v\n", hostname, err)
			continue
		}
		resolves = append(resolves, IPs...)
	}
	config.Cache.Resolves = uniqueIPs(resolves)
	saveConfig()
	fmt.Printf("Cached %d resolves.\n", len(config.Cache.Resolves))
}

// unique returns a new slice containing only the unique elements of the given slice.
func unique[S ~[]T, T comparable](s S) S {
	m := make(map[T]struct{}, len(s))
	for _, e := range s {
		m[e] = struct{}{}
	}
	r := make([]T, 0, len(m))
	for e := range m {
		r = append(r, e)
	}
	return r
}

// uniqueIPs returns a new slice containing only the unique elements of the given slice of net.IP.
func uniqueIPs(s []net.IP) []net.IP {
	m := make(map[string]int, len(s))
	for i, e := range s {
		m[e.String()] = i
	}
	r := make([]net.IP, 0, len(m))
	for _, v := range m {
		r = append(r, s[v])
	}
	return r
}
