package urlutil

import (
	"net/url"
	"strings"
)

// ResolveRelativeURL resolves href against baseURL while preserving absolute href values.
func ResolveRelativeURL(baseURL, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	rel, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(rel).String()
}
