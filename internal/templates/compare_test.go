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
