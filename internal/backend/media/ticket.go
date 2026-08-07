package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ticketTTL is long enough to cover a typical playback session: a seek past
// the ticket's expiry re-requests the URI with an expired ticket and 401s with
// no re-mint path, so this errs toward session-length rather than a short,
// security-minded expiry. (C-1)
const ticketTTL = 6 * time.Hour

// PathPrefix is the URL path under which downloaded media is served (GET /media/{id}).
const PathPrefix = "/media/"

// MediaURL builds the relative playable URL for a locally-downloaded video,
// appending a signed access ticket when token is non-empty. The backend/transport
// call this so the rule for "how a downloaded file is addressed" lives here in
// the media package instead of being reconstructed inside a transport handler.
func MediaURL(token, videoID string) string {
	uri := PathPrefix + videoID
	if token != "" {
		uri += "?t=" + MintTicket(token, videoID)
	}
	return uri
}

// MintTicket returns a signed URL ticket for the given video ID.
// The ticket is "<hmac>.<expiry>" where expiry is a Unix timestamp.
func MintTicket(token, videoID string) string {
	expiry := time.Now().Add(ticketTTL).Unix()
	sig := ticketSig(token, videoID, expiry)
	return sig + "." + strconv.FormatInt(expiry, 10)
}

// ValidateTicket returns true if the ticket is a valid, unexpired HMAC for videoID.
func ValidateTicket(token, videoID, ticket string) bool {
	if token == "" {
		return false
	}
	dot := strings.LastIndexByte(ticket, '.')
	if dot < 0 {
		return false
	}
	sig := ticket[:dot]
	expiry, err := strconv.ParseInt(ticket[dot+1:], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	want := ticketSig(token, videoID, expiry)
	return hmac.Equal([]byte(sig), []byte(want))
}

func ticketSig(token, videoID string, expiry int64) string {
	h := hmac.New(sha256.New, ticketKey(token))
	_, _ = fmt.Fprintf(h, "%s|%d", videoID, expiry)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ticketKey derives a media-ticket signing key from the API bearer token so the
// two secrets are cryptographically separated: a leaked ticket signature can't
// be worked backwards into the bearer token. This is an HKDF-style single-step
// key derivation using HMAC as the PRF, keyed by a fixed context label. (L-6)
func ticketKey(token string) []byte {
	m := hmac.New(sha256.New, []byte("yt-tui/media-ticket/v1"))
	_, _ = m.Write([]byte(token))
	return m.Sum(nil)
}
