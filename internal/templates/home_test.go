package templates

import (
	"testing"

	"github.com/philspins/open-democracy/internal/opennorth"
)

func TestHomeRepresentativeHeading_IncludesOffice(t *testing.T) {
	got := homeRepresentativeHeading(opennorth.Representative{
		Name:          "Teresa J. Armstrong",
		ElectedOffice: "MPP",
	}, "Your current provincial representative", "Provincial representative")
	if got != "Teresa J. Armstrong (MPP)" {
		t.Fatalf("homeRepresentativeHeading() = %q, want %q", got, "Teresa J. Armstrong (MPP)")
	}
}

func TestHomeRepresentativeHeading_FallsBackWhenMissingData(t *testing.T) {
	got := homeRepresentativeHeading(opennorth.Representative{}, "Your current provincial representative", "Provincial representative")
	if got != "Your current provincial representative (Provincial representative)" {
		t.Fatalf("homeRepresentativeHeading() = %q, want %q", got, "Your current provincial representative (Provincial representative)")
	}
}
