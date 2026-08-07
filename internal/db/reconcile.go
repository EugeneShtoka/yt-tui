package db

import (
	"context"
	"fmt"
	"os"
)

// Download/delete history event types. These mirror the strings the downloader
// writes ("download "+type, downloader.go) and the delete path writes; they are
// the events whose latest occurrence per video decides whether history still
// considers a video downloaded.
const (
	evtDownloadVideo = "download video"
	evtDownloadAudio = "download audio"
	evtDelete        = "delete"
)

// reconcileDownloads closes dangling download/delete lifecycles left by
// out-of-band changes — a file deleted or a disk moved outside the app. It runs
// once at startup, DB-side (not TUI-side): in remote mode the files and DB live
// on the daemon host, so only the layer that opened the DB can stat them.
//
// Algorithm (single pass, see the plan's "Startup reconciliation" section):
//   - Set A = all local_videos rows. For each, os.Stat the file:
//   - os.IsNotExist → append a delete event ("auto: file missing") then drop
//     the row (its file is already gone).
//   - any other stat error (permission denied, unmounted drive) → leave
//     untouched; never prune on an ambiguous error or an offline disk wipes
//     the library.
//   - file present → healthy; backfill file_size from the same stat when the
//     row still has none (Phase 11: rows written before the size column).
//   - Set B = video_ids whose latest download/delete event is a download. For
//     each id in B not in A (history says downloaded, no row) → append a delete
//     event ("auto: orphaned record") to close the lifecycle.
//
// Both auto-closes reuse the existing "delete" event string so they render
// identically in the History tab, distinguished only by their details.
func (d *DB) reconcileDownloads(ctx context.Context) error {
	locals, err := d.localVideoFiles(ctx)
	if err != nil {
		return err
	}

	inA := make(map[string]bool, len(locals))
	for id := range locals {
		inA[id] = true
	}

	// Step A: stat each local file; prune only on definitive absence.
	for id, lf := range locals {
		if lf.path == "" {
			continue
		}
		info, statErr := os.Stat(lf.path)
		if statErr == nil {
			// Healthy. Backfill file_size once for pre-Phase-11 rows; the stat
			// is already in hand, so this adds no extra syscall.
			if lf.size == 0 {
				if err = d.setLocalVideoSize(ctx, id, info.Size()); err != nil {
					return fmt.Errorf("reconcileDownloads backfill size %s: %w", id, err)
				}
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			continue // ambiguous — never prune
		}
		if err = d.AddHistory(ctx, id, evtDelete, "auto: file missing"); err != nil {
			return fmt.Errorf("reconcileDownloads close missing %s: %w", id, err)
		}
		if err = d.DeleteLocalVideo(ctx, id); err != nil {
			return fmt.Errorf("reconcileDownloads prune %s: %w", id, err)
		}
		delete(inA, id)
	}

	// Step B: history says downloaded but no local row → close the lifecycle.
	dangling, err := d.danglingDownloadIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range dangling {
		if inA[id] {
			continue
		}
		if err = d.AddHistory(ctx, id, evtDelete, "auto: orphaned record"); err != nil {
			return fmt.Errorf("reconcileDownloads close orphan %s: %w", id, err)
		}
	}
	return nil
}

// localFile is one local_videos row's on-disk projection: its path and the
// size the DB currently records (0 = not yet backfilled).
type localFile struct {
	path string
	size int64
}

// localVideoFiles returns every local_videos row as id → localFile (Set A).
func (d *DB) localVideoFiles(ctx context.Context) (map[string]localFile, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, file_path, COALESCE(file_size, 0) FROM local_videos`)
	if err != nil {
		return nil, fmt.Errorf("reconcileDownloads query local: %w", err)
	}
	defer rows.Close()
	out := make(map[string]localFile)
	for rows.Next() {
		var id string
		var lf localFile
		if err := rows.Scan(&id, &lf.path, &lf.size); err != nil {
			return nil, fmt.Errorf("reconcileDownloads scan local: %w", err)
		}
		out[id] = lf
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reconcileDownloads local rows: %w", err)
	}
	return out, nil
}

// setLocalVideoSize records the on-disk byte size for a downloaded video.
func (d *DB) setLocalVideoSize(ctx context.Context, id string, size int64) error {
	if _, err := d.sql.ExecContext(ctx, `UPDATE local_videos SET file_size=? WHERE id=?`, size, id); err != nil {
		return fmt.Errorf("setLocalVideoSize: %w", err)
	}
	return nil
}

// danglingDownloadIDs returns the video_ids whose latest event among the
// download/delete set is a download — i.e. history still considers them
// downloaded (Set B). Latest is by id (autoincrement, monotonic with
// insertion); NULL/empty video_ids are skipped.
func (d *DB) danglingDownloadIDs(ctx context.Context) ([]string, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT h.video_id
		FROM history h
		JOIN (
			SELECT video_id, MAX(id) AS latest
			FROM history
			WHERE event_type IN (?, ?, ?)
			  AND video_id IS NOT NULL AND video_id != ''
			GROUP BY video_id
		) m ON m.latest = h.id
		WHERE h.event_type LIKE 'download %'
	`, evtDownloadVideo, evtDownloadAudio, evtDelete)
	if err != nil {
		return nil, fmt.Errorf("reconcileDownloads query history: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reconcileDownloads scan history: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reconcileDownloads history rows: %w", err)
	}
	return ids, nil
}
