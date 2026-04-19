package provincial

import "net/http"

func CrawlBritishColumbiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlBritishColumbiaVotes(indexURL, legislature, session, client)
}
