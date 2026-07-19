# Fix Plan — July 19 2026

## Issues & Root Causes

### 1. History column title offset
Caused by ColDuration being 2 chars too small (same root as issue 6).
With ColDuration = maxLen*2+1 (too small), titleW is 2 chars too wide →
columns overflow on the right → visual misalignment.
Fix: same as issue 6 (ColDuration = maxLen*2+3). ✅ DONE (render.go)

### 2. Search — two tables (channels | videos) with ToggleMode
Redesign search.go:
- Remove `resultsTable` (mixed channels+videos)
- Add `chTable table.Model` (channels only)
- Add `vidTable table.Model` (videos only, renamed from resultsTable)
- Add `srchOnVideos bool` (which table has focus; default false = channels)
- Channel table columns: #(4), indicator(3), Name(rest)
- Video table columns: same as computeVideoColumns(width, true)
- ToggleMode key (m) switches srchOnVideos
- Heights: both shown simultaneously; channels get min(nCh, avail/3), videos get rest
- Nav keys route to focused table
- DrillDown on channels table → drill into channel
- Play/Download on videos table → as before
- srchCurrentVideo() uses vidTable.Cursor() directly

### 3. Fix offset between channels and videos in search
Caused by channels using styles.Warning.Render("ch") (2 visual chars) in a 3-wide
indicator column, + ANSI reset mid-row clearing selected BG.
Fix: with separate tables (issue 2), each table has consistent indicators.
Channel table uses plain "   " (3 spaces) or "ch " indicator.

### 4. Fix selected row background doesn't span whole line (watched videos)
Root cause: styles.Dim.Render(title) = "\033[2m{title}\033[0m"
The trailing \033[0m full-reset clears the selected row's background.
After reset, Channel/Duration/Views/Date columns lose the BG highlight.
Fix: in styledTitle(), replace trailing "\033[0m" or "\033[m" with "\033[22;39m"
(reset bold/faint + fg only, keep background). Helper: swapReset(s string) string.
Also apply swapReset to history Type column: styles.Warning.Render(eventType).

### 5. Deleted downloaded video retains blue color till restart
Root cause: CancelDownload removes from queue but other tabs' localStatus map
is not refreshed → video still shows as StatusNew (bold+highlight).
Fix: in downloading.go Delete handler, emit tuipkg.RefreshPositionsMsg{} via
tea.Batch alongside the DownloadItemsChangedMsg. This causes all tabs to reload
localStatus from backend (which now has the video removed).

### 6. Duration width calc +2
Root cause: DurationWithPos should display "pos / total" (with spaces around /)
instead of "pos/total". The separator goes from 1 char to 3 chars, +2 total.
Fix: ColDuration = maxLen*2 + 3 (was maxLen*2+1). ✅ DONE (render.go)
Also change DurationWithPos to use " / " separator.

### 7. Subscriptions show channel alias
When subLoadCmd loads videos, each video has v.Channel = raw channel name.
But Channel.DisplayName() returns alias if set.
Fix:
- Add channelAliases map[string]string to Subscriptions struct
- Extend subLoadedMsg to carry channels
- In subLoadCmd, return channels alongside videos
- Build alias map from channels in subLoadedMsg handler
- Add videosWithAliases() method that copies videos with Channel field replaced
- Use videosWithAliases() everywhere toVideoRows is called in subscriptions.go

## Implementation Order
1. ✅ render.go — ColDuration = maxLen*2+3
2. render.go — DurationWithPos separator " / "
3. table_helpers.go — swapReset helper + update styledTitle
4. history.go — swapReset on Warning.Render in toHistRows
5. downloading.go — tea.Batch RefreshPositionsMsg on Delete
6. subscriptions.go — channel alias support
7. search.go — two-table refactor (biggest change)
