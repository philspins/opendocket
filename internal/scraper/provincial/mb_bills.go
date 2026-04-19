package provincial

import "net/http"

func CrawlManitobaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlManitobaBills(indexURL, legislature, session, client)
}
