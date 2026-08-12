package youtube

import (
	"strings"
	"testing"
)

// TestToVideoChannelFallbacks exercises toVideo's channel-name fallback chain
// (channel → playlist_channel → uploader → playlist_uploader) and the
// channel_id extraction from channel_url — paths the flat-listing fixtures in
// fetcher_test.go never hit (L-10). Driven through parseVideoLines, which
// requires id, title and a non-zero view_count.
func TestToVideoChannelFallbacks(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantChan   string
		wantChanID string
	}{
		{
			name:     "uploader fallback when channel/playlist_channel empty",
			line:     `{"id":"v1","title":"T","view_count":5,"uploader":"UpChan"}`,
			wantChan: "UpChan",
		},
		{
			name:     "playlist_channel preferred over uploader",
			line:     `{"id":"v2","title":"T","view_count":5,"playlist_channel":"PlChan","uploader":"UpChan"}`,
			wantChan: "PlChan",
		},
		{
			name:     "playlist_uploader is the last fallback",
			line:     `{"id":"v3","title":"T","view_count":5,"playlist_uploader":"PlUp"}`,
			wantChan: "PlUp",
		},
		{
			name:     "channel wins over every fallback",
			line:     `{"id":"v4","title":"T","view_count":5,"channel":"RealChan","uploader":"UpChan"}`,
			wantChan: "RealChan",
		},
		{
			name:       "channel_id extracted from channel_url when channel_id absent",
			line:       `{"id":"v5","title":"T","view_count":5,"channel_url":"https://www.youtube.com/channel/UCabc123/videos"}`,
			wantChanID: "UCabc123",
		},
		{
			name:       "explicit channel_id wins over channel_url",
			line:       `{"id":"v6","title":"T","view_count":5,"channel_id":"UCexplicit","channel_url":"https://www.youtube.com/channel/UCfromurl"}`,
			wantChanID: "UCexplicit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := parseVideoLines(strings.NewReader(tt.line))
			if err != nil {
				t.Fatalf("parseVideoLines: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("parsed %d videos, want 1", len(got))
			}
			if tt.wantChan != "" && got[0].Channel != tt.wantChan {
				t.Errorf("Channel = %q, want %q", got[0].Channel, tt.wantChan)
			}
			if tt.wantChanID != "" && got[0].ChannelID != tt.wantChanID {
				t.Errorf("ChannelID = %q, want %q", got[0].ChannelID, tt.wantChanID)
			}
		})
	}
}
