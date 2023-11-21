package cmd

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"weibo-image-hound/internal/hound"
	"weibo-image-hound/internal/weibo"
)

// huntCmd represents the hunt command
var huntCmd = &cobra.Command{
	Use:   "hunt [URL] [flags]",
	Short: "Hunt for an uncensored Weibo image, given its URL",
	Long: `Hunt for an uncensored Weibo image, given its URL. 
Example: weibo-image-hound hunt https://wx1.sinaimg.cn/mw690/006UeiBSgy1hjnwewgeclj30u01400xm.jpg`,
	Run: hunt,
}

func init() {
	rootCmd.AddCommand(huntCmd)
	huntCmd.Flags().StringP("output", "o", "", "Output file path.")
}

func hunt(cmd *cobra.Command, args []string) {
	dir, filename, err := parseOutputPath(cmd.Flag("output").Value.String())
	if err != nil {
		panic(fmt.Errorf("failed to parse output path: %w", err))
	}

	URL := args[0]
	u, err := parseURL(URL)
	if err != nil {
		panic(fmt.Errorf("invalid Weibo image URL: %w", err))
	}

	IPs := config.Cache.Resolves
	if len(IPs) == 0 {
		fmt.Println("No cached resolves found, please run `weibo-image-hound cache` first")
		return
	}
	fmt.Printf("Using %d cached resolves.\n", len(IPs))

	URLs, err := weibo.GenerateURLsOfAllQualities(URL)
	if err != nil {
		URLs = []string{URL}
	}
	var result hound.Result
	bar := progressbar.Default(int64(len(URLs)) * int64(len(IPs)))
urls:
	for _, URL = range URLs {
		fmt.Printf("Started hunting for %s\n", URL)
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan hound.Result, len(IPs))
		go hound.Hunt(ctx, ch, URL, u.Port(), IPs, nil)
		for range IPs {
			result = <-ch
			_ = bar.Add(1)
			if result.Err != nil {
				fmt.Printf("[FAILED] %s | %v\n", result.IP.String(), result.Err)
				continue
			}
			if result.Status != http.StatusOK {
				//fmt.Printf("[FAILED] %s | HTTP %d\n", result.IP.String(), result.Status)
				continue
			}
			// succeeded
			cancel()
			break urls
		}
		cancel()
		fmt.Printf("[FAILED] All failed for %s\n", URL)
	}

	fmt.Printf("[SUCCESS] %s | %s | %d\n", URL, result.IP.String(), len(result.Body))
	// write to file
	if filename == "." || filename == "/" { // build urls filename when not specified
		filename = u.Path[strings.LastIndex(u.Path, "/")+1:]
		if strings.LastIndex(filename, ".") == -1 { // no extension or empty
			mimeType := result.Headers.Get("content-type")
			if mimeType == "" {
				mimeType = http.DetectContentType(result.Body)
			}
			var fileExt string
			switch mimeType {
			case "image/jpeg": // avoid using ".jfif" from mime.ExtensionsByType
				fileExt = ".jpg"
			case "application/octet-stream":
				fileExt = ".bin"
			default:
				if exts, err := mime.ExtensionsByType(mimeType); err != nil || len(exts) == 0 {
					fileExt = ".bin"
				} else {
					fileExt = exts[0]
				}
			}
			if filename == "" { // use current unix timestamp
				filename = strconv.FormatInt(time.Now().Unix(), 10)
			}
			filename += fileExt
		}
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, result.Body, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Saved %s to %s\n", URL, path)
}

// parseURL parses a URL string and returns an url.URL struct, with all needed stuff fixed up.
func parseURL(URL string) (*url.URL, error) {
	if URL == "" {
		return nil, fmt.Errorf("empty")
	}
	if strings.HasPrefix(URL, "//") {
		URL = "https:" + URL
	}
	u, err := url.Parse(URL)
	if err != nil {
		return nil, err
	}
	if u.Port() == "" {
		if u.Scheme == "https" {
			u.Host += ":443"
		} else if u.Scheme == "http" {
			u.Host += ":80"
		} else {
			return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
		}
	} else {
		p, err := strconv.ParseUint(u.Port(), 10, 16)
		if err != nil || p == 0 || p > 65535 {
			return nil, fmt.Errorf("invalid port \"%s\": %w", u.Port(), err)
		}
	}
	return u, nil
}

// parseOutputPath parses a path string and returns the absolute path to the directory, and filename.
// If the given path points to a directory, the filename will be "/".
func parseOutputPath(path string) (dir string, filename string, err error) {
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}
	dir, filename = filepath.Split(path)
	if f, err := os.Stat(dir); err != nil {
		return "", "", err
	} else if !f.IsDir() {
		return "", "", fmt.Errorf("directory not exists")
	}
	if f, err := os.Stat(filepath.Join(dir, filename)); err == nil && f.IsDir() {
		dir = filepath.Join(dir, filename)
		filename = "/"
	}
	return dir, filename, nil
}
