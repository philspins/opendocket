package provincial

import "net/http"

func CrawlOntarioBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlOntarioBills(indexURL, legislature, session, client)
}

func crawlOntarioBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.ola.org/en/legislative-business"
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "on", legislature, session, "ontario", client, genericBillLinkRe)
}
