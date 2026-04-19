package provincial

import "net/http"

func CrawlNewBrunswickBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNewBrunswickBills(indexURL, legislature, session, client)
}
