package provincial

import "net/http"

func CrawlNewBrunswickVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNewBrunswickVotes(indexURL, legislature, session, client)
}
