package provincial

import "net/http"

func CrawlOntarioBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlOntarioBills(indexURL, legislature, session, client)
}
