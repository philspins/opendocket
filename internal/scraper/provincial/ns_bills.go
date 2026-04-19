package provincial

import "net/http"

func CrawlNovaScotiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNovaScotiaBills(indexURL, legislature, session, client)
}
