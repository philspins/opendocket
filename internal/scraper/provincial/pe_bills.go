package provincial

import "net/http"

func CrawlPrinceEdwardIslandBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlPrinceEdwardIslandBills(indexURL, legislature, session, client)
}
