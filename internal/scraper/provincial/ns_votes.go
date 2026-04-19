package provincial

import "net/http"

func CrawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNovaScotiaVotes(indexURL, legislature, session, client)
}
