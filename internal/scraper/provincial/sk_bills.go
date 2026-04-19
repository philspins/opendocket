package provincial

import "net/http"

func CrawlSaskatchewanBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlSaskatchewanBills(indexURL, legislature, session, client)
}
