package weibo

import (
	"fmt"
	"regexp"
)

var (
	patternImageURL = regexp.MustCompile(`(?:https?://)?([\da-zA-Z\-.]+\.sinaimg\.cn)/.+/([\da-zA-Z]+\.(?:jpg|png|gif))`)
	qualities       = []string{"mw2000", "woriginal", "large", "orj1080", "mw1024", "orj960", "sti960", "wapb720", "mw690", "orj480", "bmiddle", "wap360", "thumbnail", "thumb180", "wap180", "small", "square"}
)

func GenerateURLsOfAllQualities(URL string) ([]string, error) {
	m := patternImageURL.FindStringSubmatch(URL)
	if len(m) != 3 || m[1] == "" || m[2] == "" {
		return nil, fmt.Errorf("invalid Weibo image URL")
	}

	URLs := make([]string, len(qualities))
	for i, q := range qualities {
		URLs[i] = fmt.Sprintf("https://%s/%s/%s", m[1], q, m[2])
	}
	return URLs, nil
}
