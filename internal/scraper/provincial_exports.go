package scraper

import "github.com/philspins/open-democracy/internal/scraper/provincial"

type ProvincialBillStub = provincial.ProvincialBillStub
type ProvincialMemberVote = provincial.ProvincialMemberVote
type ProvincialDivisionResult = provincial.ProvincialDivisionResult

const (
	OntarioVPIndexURL       = provincial.OntarioVPIndexURL
	OntarioParliament       = provincial.OntarioParliament
	OntarioSession          = provincial.OntarioSession
	SaskatchewanArchiveURL  = provincial.SaskatchewanArchiveURL
	SaskatchewanLegislature = provincial.SaskatchewanLegislature
	SaskatchewanSession     = provincial.SaskatchewanSession
)

var (
	ExtractProvincialBillNumber       = provincial.ExtractProvincialBillNumber
	ProvincialBillID                  = provincial.ProvincialBillID
	CrawlProvincialBillsFromIndex     = provincial.CrawlProvincialBillsFromIndex
	CrawlAlbertaBills                 = provincial.CrawlAlbertaBills
	CrawlBritishColumbiaBills         = provincial.CrawlBritishColumbiaBills
	CrawlManitobaBills                = provincial.CrawlManitobaBills
	CrawlNewBrunswickBills            = provincial.CrawlNewBrunswickBills
	CrawlNewfoundlandAndLabradorBills = provincial.CrawlNewfoundlandAndLabradorBills
	CrawlNovaScotiaBills              = provincial.CrawlNovaScotiaBills
	CrawlOntarioBills                 = provincial.CrawlOntarioBills
	CrawlPrinceEdwardIslandBills      = provincial.CrawlPrinceEdwardIslandBills
	CrawlQuebecBills                  = provincial.CrawlQuebecBills
	CrawlSaskatchewanBills            = provincial.CrawlSaskatchewanBills

	ProvincialDivisionID                 = provincial.ProvincialDivisionID
	CrawlOntarioVPSittingDates           = provincial.CrawlOntarioVPSittingDates
	OntarioVPDayURL                      = provincial.OntarioVPDayURL
	CrawlOntarioVPDay                    = provincial.CrawlOntarioVPDay
	CrawlSaskatchewanMinutesLinks        = provincial.CrawlSaskatchewanMinutesLinks
	CrawlSaskatchewanMinutes             = provincial.CrawlSaskatchewanMinutes
	ParliamentOrdinalForTest             = provincial.ParliamentOrdinalForTest
	ParseNewBrunswickPDFDivisionsForTest = provincial.ParseNewBrunswickPDFDivisionsForTest
	ParsePDFDivisionsYeasNaysForTest     = provincial.ParsePDFDivisionsYeasNaysForTest
	ParseAlbertaVPDivisionsForTest       = provincial.ParseAlbertaVPDivisionsForTest
	ParseBCVotesDivisionsForTest         = provincial.ParseBCVotesDivisionsForTest
	ParseManitobaAyeNayDivisionsForTest  = provincial.ParseManitobaAyeNayDivisionsForTest
	ParseNLJournalDivisionsForTest       = provincial.ParseNLJournalDivisionsForTest
	ParsePEIJournalDivisionsForTest      = provincial.ParsePEIJournalDivisionsForTest
	CrawlAlbertaVotes                    = provincial.CrawlAlbertaVotes
	CrawlBritishColumbiaVotes            = provincial.CrawlBritishColumbiaVotes
	CrawlManitobaVotes                   = provincial.CrawlManitobaVotes
	CrawlQuebecVotes                     = provincial.CrawlQuebecVotes
	CrawlNewBrunswickVotes               = provincial.CrawlNewBrunswickVotes
	CrawlNewfoundlandAndLabradorVotes    = provincial.CrawlNewfoundlandAndLabradorVotes
	CrawlNovaScotiaVotes                 = provincial.CrawlNovaScotiaVotes
	CrawlPrinceEdwardIslandVotes         = provincial.CrawlPrinceEdwardIslandVotes
	CrawlGenericProvincialVotes          = provincial.CrawlGenericProvincialVotes
)
