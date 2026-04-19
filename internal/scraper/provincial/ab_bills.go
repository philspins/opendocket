package provincial

import "net/http"

func CrawlAlbertaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlAlbertaBills(indexURL, legislature, session, client)
}
