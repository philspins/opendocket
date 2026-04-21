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
