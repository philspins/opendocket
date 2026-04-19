package provincial

import "net/http"

func CrawlManitobaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlManitobaVotes(indexURL, legislature, session, client)
}
