package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/philspins/open-democracy/internal/store"
)

func TestCompareMPs_RendersDropdownFilters(t *testing.T) {
	members := []store.MemberRow{
		{ID: "f1", Name: "Alex Federal", Party: "Liberal"},
		{ID: "f2", Name: "Blake Federal", Party: "Conservative"},
	}

	var buf bytes.Buffer
	err := CompareMPs(
		store.ParliamentStatus{},
		members,
		members[0],
		members[1],
		"federal",
		"",
		"",
		[]string{"Ontario"},
		[]string{"Conservative", "Liberal"},
		0,
		0,
		nil,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render compare: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "type=\"text\" name=\"a\"") || strings.Contains(html, "type=\"text\" name=\"b\"") {
		t.Fatalf("expected member selectors to be dropdowns, got text inputs")
	}
	for _, needle := range []string{
		`<select name="a"`,
		`<select name="b"`,
		`<select name="level"`,
		`<select name="party"`,
		`onchange="this.form.submit()"`,
		`value="federal" selected`,
		`Alex Federal (Liberal)`,
		`Blake Federal (Conservative)`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("expected compare page to contain %q", needle)
		}
	}
	if strings.Contains(html, `<select name="province"`) {
		t.Fatalf("expected province selector to be hidden when level is federal")
	}
}

func TestCompareMPs_ShowsProvinceFilterForProvincial(t *testing.T) {
	members := []store.MemberRow{
		{ID: "p1", Name: "Casey Provincial", Party: "NDP"},
	}

	var buf bytes.Buffer
	err := CompareMPs(
		store.ParliamentStatus{},
		members,
		members[0],
		store.MemberRow{},
		"provincial",
		"Ontario",
		"NDP",
		[]string{"Ontario", "Quebec"},
		[]string{"NDP"},
		0,
		0,
		nil,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render compare: %v", err)
	}
	html := buf.String()

	for _, needle := range []string{
		`<select name="province"`,
		`value="provincial" selected`,
		`<option value="Ontario" selected>Ontario</option>`,
		`<option value="NDP" selected>NDP</option>`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("expected provincial compare page to contain %q", needle)
		}
	}
}

func TestCompareMPs_RendersSharedVotesTable(t *testing.T) {
	members := []store.MemberRow{
		{ID: "m1", Name: "Alex Federal", Party: "Liberal"},
		{ID: "m2", Name: "Blake Federal", Party: "Conservative"},
	}
	shared := []store.SharedVoteRow{
		{
			DivisionID:  "d1",
			Date:        "2025-01-10",
			BillID:      "b1",
			BillNumber:  "C-1",
			Description: "First reading",
			Result:      "Carried",
			Member1Vote: "Yea",
			Member2Vote: "Nay",
		},
	}

	var buf bytes.Buffer
	err := CompareMPs(
		store.ParliamentStatus{},
		members,
		members[0],
		members[1],
		"federal",
		"",
		"",
		[]string{},
		[]string{"Conservative", "Liberal"},
		1,
		1,
		shared,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render compare: %v", err)
	}
	html := buf.String()
	for _, needle := range []string{
		`id="compare-votes-section"`,
		`Votes in Common`,
		`<th class="px-4 py-2">Alex Federal</th>`,
		`<th class="px-4 py-2">Blake Federal</th>`,
		`C-1`,
		`Yea`,
		`Nay`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("expected compare shared votes section to contain %q", needle)
		}
	}
}
