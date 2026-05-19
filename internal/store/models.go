package store

type BillRow struct {
	ID               string
	Parliament       int
	Session          int
	Number           string
	Title            string
	ShortTitle       string
	BillType         string
	Chamber          string
	SponsorID        string
	SponsorName      string
	CurrentStage     string
	CurrentStatus    string
	Category         string
	SummaryAI        string
	FullTextURL      string
	LegisInfoURL     string
	IntroducedDate   string
	LastActivityDate string
}

type DivisionRow struct {
	ID          string
	Parliament  int
	Session     int
	Number      int
	Date        string
	BillID      string
	BillNumber  string
	BillTitle   string
	Description string
	Yeas        int
	Nays        int
	Paired      int
	Result      string
	Chamber     string
	SittingURL  string
}

type MemberRow struct {
	ID              string
	Name            string
	Party           string
	Riding          string
	Province        string
	Role            string
	PhotoURL        string
	Email           string
	Website         string
	Chamber         string
	Active          bool
	GovernmentLevel string // "federal" | "provincial"
	TermStart       string // ISO-8601 date (YYYY-MM-DD)
	TermEnd         string // ISO-8601 date (YYYY-MM-DD), empty means open-ended
}

type VoteRow struct {
	DivisionID     string
	Date           string
	BillID         string
	BillNumber     string
	BillTitle      string
	Description    string
	Vote           string
	Result         string
	VotedWithParty bool
	PartyMajority  string
}

type SharedVoteRow struct {
	DivisionID  string
	Date        string
	BillID      string
	BillNumber  string
	BillTitle   string
	Description string
	Result      string
	Member1Vote string
	Member2Vote string
}

type MemberStats struct {
	TotalVotes   int
	PartyLinePct int
	RebelPct     int
	MissedPct    int
}

type ParliamentStatus struct {
	Status     string // "in_session" | "on_break"
	Label      string
	Detail     string
	Parliament int
	Session    int
}

type BillFilter struct {
	Search              string
	Stage               string
	Category            string
	Chamber             string
	Level               string // "" | "federal" | "provincial"
	Province            string
	Sort                string   // "" | "date_asc" | "stage" | "category" | "auto"
	PreferredCategories []string // used when Sort="auto"
	SubscribedBillIDs   []string // used when Sort="auto" to pin subscribed bills first
	Page                int
	PerPage             int
}

type DivisionFilter struct {
	Chamber string // "" means all; otherwise matches d.chamber exactly
	Result  string // "" | "Carried" | "Negatived"
	Page    int
	PerPage int
}

type BillStageRow struct {
	Stage   string
	Chamber string
	Date    string
	Notes   string
}

type UserRow struct {
	ID                 string
	Email              string
	EmailVerified      bool
	Address            string
	FederalRidingID    string
	ProvincialRidingID string
	CreatedAt          string
	EmailDigest        string
}

type BillReactionCounts struct {
	BillID         string
	SupportCount   int
	OpposeCount    int
	NeutralCount   int
	TotalReactions int
	RefreshedAt    string
}

// CategoryScore holds an MP's voting tendency on bills in a given category.
type CategoryScore struct {
	Category string
	Total    int
	Yeas     int
	Nays     int
	YeaPct   int // 0–100, rounded
}

// MissedVoteRow represents a division that occurred during a member's term
// where the member has no recorded vote.
type MissedVoteRow struct {
	DivisionID    string
	Date          string
	BillID        string
	BillNumber    string
	BillTitle     string
	Description   string
	PartyMajority string // "Yea", "Nay", or "Split"
	Result        string
}
