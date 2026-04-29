package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/store"
)

func TestBillDetail_RendersReactionFormsForAuthenticatedUsers(t *testing.T) {
	var buf bytes.Buffer
	err := BillDetail(
		store.ParliamentStatus{},
		store.BillRow{ID: "45-1-c-47", Number: "C-47", Title: "An Act"},
		nil,
		nil,
		store.BillReactionCounts{},
		true,
		false,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render bill detail: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, `action="/api/react"`) {
		t.Fatalf("expected authenticated users to see reaction forms")
	}
	if strings.Contains(html, "login to share your opinion") {
		t.Fatalf("did not expect login prompt for authenticated users")
	}
}

func TestBillDetail_RendersLoginPromptForGuests(t *testing.T) {
	var buf bytes.Buffer
	err := BillDetail(
		store.ParliamentStatus{},
		store.BillRow{ID: "45-1-c-47", Number: "C-47", Title: "An Act"},
		nil,
		nil,
		store.BillReactionCounts{},
		false,
		false,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render bill detail: %v", err)
	}

	html := buf.String()
	if strings.Contains(html, `action="/api/react"`) {
		t.Fatalf("did not expect reaction forms for guests")
	}
	if strings.Contains(html, "absolute inset-0") {
		t.Fatalf("did not expect overlay prompt for guests")
	}
	if !strings.Contains(html, "login to share your opinion") {
		t.Fatalf("expected hover tooltip prompt for guests")
	}
	if !strings.Contains(html, "group-hover:opacity-100") {
		t.Fatalf("expected guest tooltip to appear on hover")
	}
	if !strings.Contains(html, "text-sm text-white") {
		t.Fatalf("expected guest tooltip text size to match summary text")
	}
	if !strings.Contains(html, "opacity-40") {
		t.Fatalf("expected greyed out reaction controls for guests")
	}
}

func TestBillDetail_ShowsSubscribedState(t *testing.T) {
	var buf bytes.Buffer
	err := BillDetail(
		store.ParliamentStatus{},
		store.BillRow{ID: "45-1-c-47", Number: "C-47", Title: "An Act"},
		nil,
		nil,
		store.BillReactionCounts{},
		true,
		true,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render bill detail: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Subscribed") {
		t.Fatalf("expected subscribed button text for subscribed bill")
	}
	if !strings.Contains(html, `action="/api/subscribe-bill"`) {
		t.Fatalf("expected subscribe form for authenticated user")
	}
}

func TestBillDetail_ShowsSubscribeButtonForUnsubscribed(t *testing.T) {
	var buf bytes.Buffer
	err := BillDetail(
		store.ParliamentStatus{},
		store.BillRow{ID: "45-1-c-47", Number: "C-47", Title: "An Act"},
		nil,
		nil,
		store.BillReactionCounts{},
		true,
		false,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render bill detail: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `action="/api/subscribe-bill"`) {
		t.Fatalf("expected subscribe form for authenticated user")
	}
}

func TestBillDetail_GuestSeesDisabledSubscribeButton(t *testing.T) {
	var buf bytes.Buffer
	err := BillDetail(
		store.ParliamentStatus{},
		store.BillRow{ID: "45-1-c-47", Number: "C-47", Title: "An Act"},
		nil,
		nil,
		store.BillReactionCounts{},
		false,
		false,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render bill detail: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, `action="/api/subscribe-bill"`) {
		t.Fatalf("did not expect subscribe form for guest")
	}
	if !strings.Contains(html, "Login to subscribe") {
		t.Fatalf("expected login prompt for subscribe button for guest")
	}
}
