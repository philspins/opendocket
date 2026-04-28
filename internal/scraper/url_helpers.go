package scraper

import (
	"github.com/philspins/opendocket/internal/urlutil"
)

func resolveRelativeURL(baseURL, href string) string {
	return urlutil.ResolveRelativeURL(baseURL, href)
}
