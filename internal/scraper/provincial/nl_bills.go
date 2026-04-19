package provincial

import "net/http"

func CrawlNewfoundlandAndLabradorBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNewfoundlandAndLabradorBills(indexURL, legislature, session, client)
}
