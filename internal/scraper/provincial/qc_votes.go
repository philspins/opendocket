package provincial

import "net/http"

func CrawlQuebecVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlQuebecVotes(indexURL, legislature, session, client)
}
