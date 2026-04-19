package provincial

import "net/http"

func CrawlQuebecBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlQuebecBills(indexURL, legislature, session, client)
}
