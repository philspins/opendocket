package provincial

import "net/http"

func CrawlBritishColumbiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlBritishColumbiaBills(indexURL, legislature, session, client)
}
