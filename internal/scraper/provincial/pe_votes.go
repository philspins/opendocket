package provincial

import "net/http"

func CrawlPrinceEdwardIslandVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlPrinceEdwardIslandVotes(indexURL, legislature, session, client)
}
