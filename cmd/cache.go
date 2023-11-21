package cmd

import (
	"fmt"
	"net"
	"os"
	"sync"

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
Example: weibo-image-hound cache -p globalping -f`,
	Run: cache,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.Flags().StringP("provider", "p", "globalping", "probe provider to use")
	cacheCmd.Flags().BoolP("force", "f", false, "force overwrite existing cached resolves")
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

	// cache resolves
	locations, err := provider.Locations()
	if err != nil {
		panic(fmt.Errorf("failed to get locations: %w", err))
	}
	locations = unique(locations)
	fmt.Printf("Using %d locations.\n", len(locations))

	hostnames := weibo.Hostnames()
	var wg sync.WaitGroup
	ch := make(chan []net.IP, len(hostnames))
	for _, h := range hostnames {
		wg.Add(1)
		go func(hostname string) {
			defer wg.Done()
			IPs, err := provider.Resolve(hostname, locations)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to resolve \"%s\": %v\n", hostname, err)
				ch <- nil
				return
			}
			ch <- IPs
		}(h)
	}
	wg.Wait()

	resolves := config.Cache.Resolves
	if cmd.Flag("force").Changed { // force overwrite
		resolves = nil
	}
	for range hostnames {
		resolves = append(resolves, <-ch...)
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
