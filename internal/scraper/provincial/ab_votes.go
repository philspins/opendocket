package provincial

import "net/http"

func CrawlAlbertaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlAlbertaVotes(indexURL, legislature, session, client)
}
