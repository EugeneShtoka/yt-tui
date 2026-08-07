package media_test

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
)

func TestMintValidateTicket(t *testing.T) {
	ticket := media.MintTicket("tok", "vid1")
	if !media.ValidateTicket("tok", "vid1", ticket) {
		t.Error("fresh ticket should validate")
	}
}

func TestTicketWrongVideo(t *testing.T) {
	ticket := media.MintTicket("tok", "vid1")
	if media.ValidateTicket("tok", "vid2", ticket) {
		t.Error("ticket for vid1 must not validate for vid2")
	}
}

func TestTicketWrongToken(t *testing.T) {
	ticket := media.MintTicket("tok", "vid1")
	if media.ValidateTicket("other", "vid1", ticket) {
		t.Error("ticket signed with 'tok' must not validate with 'other'")
	}
}

func TestTicketPastExpiry(t *testing.T) {
	// expiry 0 = 1970-01-01, always in the past
	if media.ValidateTicket("tok", "vid", "anysig.0") {
		t.Error("past-expiry ticket must be rejected")
	}
}

func TestTicketMalformed(t *testing.T) {
	for _, bad := range []string{"", "nodot", "sig.", "sig.notanint"} {
		if media.ValidateTicket("tok", "vid", bad) {
			t.Errorf("malformed ticket %q must not validate", bad)
		}
	}
}

func TestTicketEmptyToken(t *testing.T) {
	ticket := media.MintTicket("tok", "vid")
	if media.ValidateTicket("", "vid", ticket) {
		t.Error("empty token must not validate any ticket")
	}
}
