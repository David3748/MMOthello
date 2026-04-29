package main

import (
	"path/filepath"
	"testing"
	"time"

	"mmothello/server/internal/board"
	"mmothello/server/internal/persist"
)

func TestTeamPickerBalancesLivePlayersOnly(t *testing.T) {
	picker := newTeamPicker()
	now := time.Unix(1000, 0)

	if got := picker.assign(1, now); got != 1 {
		t.Fatalf("session 1 team=%d want black", got)
	}
	picker.markPlayed(1, now)
	if got := picker.assign(2, now.Add(10*time.Second)); got != 2 {
		t.Fatalf("session 2 team=%d want white to balance live black", got)
	}
	picker.markPlayed(2, now.Add(10*time.Second))

	black, white := picker.liveCounts(now.Add(20 * time.Second))
	if black != 1 || white != 1 {
		t.Fatalf("live counts=(%d,%d) want (1,1)", black, white)
	}

	black, white = picker.liveCounts(now.Add(90 * time.Second))
	if black != 0 || white != 0 {
		t.Fatalf("expired live counts=(%d,%d) want (0,0)", black, white)
	}

	if got := picker.assign(3, now.Add(90*time.Second)); got != 1 {
		t.Fatalf("historical players should not affect assignment, got team=%d", got)
	}
}

func TestRestoreFromDiskReplaysWALPlacements(t *testing.T) {
	dir := t.TempDir()
	base := board.NewBoard()
	_ = base.Set(10, 10, board.CellBlack)
	_ = base.Set(11, 10, board.CellWhite)
	base.RecountChunks()

	ts := time.UnixMilli(1700000000123)
	base.LockAllRead()
	data := encodeBoard(base)
	base.UnlockAllRead()
	if err := persist.WriteSnapshot(dir, ts, data); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	wal, err := persist.OpenWAL(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	if err := wal.Append(persist.WALEntry{
		SessionID: 1,
		X:         12,
		Y:         10,
		Team:      uint8(board.CellBlack),
		TS:        ts.UnixMilli() + 1,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	restored := board.NewBoard()
	if err := restoreFromDisk(restored, dir); err != nil {
		t.Fatalf("restoreFromDisk: %v", err)
	}
	if restored.Get(10, 10) != board.CellBlack ||
		restored.Get(11, 10) != board.CellBlack ||
		restored.Get(12, 10) != board.CellBlack {
		t.Fatalf("WAL placement was not replayed")
	}
}
