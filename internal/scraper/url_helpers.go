package scraper

import (
	"github.com/philspins/open-democracy/internal/urlutil"
)

func resolveRelativeURL(baseURL, href string) string {
	return urlutil.ResolveRelativeURL(baseURL, href)
}
