package provincial

import "net/http"

func CrawlNewfoundlandAndLabradorVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNewfoundlandAndLabradorVotes(indexURL, legislature, session, client)
}
