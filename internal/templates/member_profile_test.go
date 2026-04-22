package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/philspins/open-democracy/internal/store"
)

func TestMemberProfile_ReordersEngageAndAddsVotePagination(t *testing.T) {
	member := store.MemberRow{
		ID:              "member-1",
		Name:            "Jane Doe",
		Party:           "Example Party",
		GovernmentLevel: "federal",
	}
	votes := make([]store.VoteRow, 21)
	for i := range votes {
		votes[i] = store.VoteRow{
			DivisionID:  "div",
			Date:        "2026-01-01",
			Description: "Vote",
			Vote:        "Yea",
			Result:      "Passed",
		}
	}

	var buf bytes.Buffer
	if err := MemberProfile(store.ParliamentStatus{}, member, votes, store.MemberStats{}, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render member profile: %v", err)
	}
	html := buf.String()

	if strings.Index(html, "Recent Votes") > strings.Index(html, "Engage With This MP") {
		t.Fatalf("expected Recent Votes section before Engage With This MP section")
	}
	if !strings.Contains(html, "id=\"member-votes-pagination\"") {
		t.Fatalf("expected member votes pagination container to be rendered")
	}
	if !strings.Contains(html, "id=\"member-votes-prev\"") || !strings.Contains(html, "id=\"member-votes-next\"") {
		t.Fatalf("expected prev/next pagination buttons to be rendered")
	}
	if !strings.Contains(html, "id=\"member-votes-page-size\"") {
		t.Fatalf("expected page-size selector to be rendered")
	}
	for _, size := range []string{"5", "10", "20", "50"} {
		if !strings.Contains(html, "option value=\""+size+"\"") {
			t.Fatalf("expected page-size option %s to be rendered", size)
		}
	}
	if !strings.Contains(html, "option value=\"10\" selected") {
		t.Fatalf("expected 10 to be the default selected page-size option")
	}
	for _, placeholder := range []string{"{ prefix }", "{ rowSelector }"} {
		if strings.Contains(html, placeholder) {
			t.Fatalf("expected member profile output to not contain unresolved placeholder %q", placeholder)
		}
	}
}

func TestMemberProfile_UsesConsistentRedStylingForNaysAndRebel(t *testing.T) {
	member := store.MemberRow{
		ID:              "member-1",
		Name:            "Jane Doe",
		Party:           "Example Party",
		GovernmentLevel: "federal",
	}
	votes := []store.VoteRow{{
		DivisionID:     "div",
		Date:           "2026-01-01",
		Description:    "Vote",
		Vote:           "Nay",
		Result:         "Failed",
		PartyMajority:  "Yea",
		VotedWithParty: false,
	}}
	catScores := []store.CategoryScore{{
		Category: "Economy",
		Yeas:     1,
		Nays:     2,
		YeaPct:   33,
	}}

	var buf bytes.Buffer
	if err := MemberProfile(store.ParliamentStatus{}, member, votes, store.MemberStats{}, catScores).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render member profile: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "bg-red-500") {
		t.Fatalf("expected bold red background for voting-by-category bar")
	}
	if !strings.Contains(html, "text-red-600 text-xs\">✗ rebel") {
		t.Fatalf("expected rebel marker to use matching red tone")
	}
}
