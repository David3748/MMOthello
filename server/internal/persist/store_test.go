package persist

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSnapshotWriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	ts := time.Unix(1700000010, 0)
	wantData := []byte{1, 2, 3, 4, 5}

	if err := WriteSnapshot(dir, ts, wantData); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	got, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadLatestSnapshot: %v", err)
	}
	if got.Meta.TimestampUnix != ts.Unix() {
		t.Fatalf("timestamp mismatch: got=%d want=%d", got.Meta.TimestampUnix, ts.Unix())
	}
	if !reflect.DeepEqual(got.Data, wantData) {
		t.Fatalf("snapshot bytes mismatch: got=%v want=%v", got.Data, wantData)
	}
}

func TestSnapshotReplayConsistency(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")
	base := []byte{0, 0, 0, 0}
	ts0 := time.Unix(1700000000, 0)

	if err := WriteSnapshot(dir, ts0, base); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	wal, err := OpenWAL(walPath)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	entries := []WALEntry{
		{SessionID: 1, X: 0, Y: 0, TS: ts0.Unix() + 1},
		{SessionID: 2, X: 2, Y: 0, TS: ts0.Unix() + 2},
		{SessionID: 3, X: 3, Y: 0, TS: ts0.Unix() + 3},
	}
	for _, e := range entries {
		if err := wal.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	snap, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadLatestSnapshot: %v", err)
	}
	gotState := append([]byte(nil), snap.Data...)

	if err := ReplayWAL(walPath, snap.Meta.TimestampUnix, func(e WALEntry) error {
		idx := int(e.X)
		if idx < len(gotState) {
			gotState[idx] = byte(e.SessionID)
		}
		return nil
	}); err != nil {
		t.Fatalf("ReplayWAL: %v", err)
	}

	wantState := []byte{1, 0, 2, 3}
	if !reflect.DeepEqual(gotState, wantState) {
		t.Fatalf("state mismatch: got=%v want=%v", gotState, wantState)
	}
}

func TestReplayIgnoresPartialTailRecord(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	wal, err := OpenWAL(walPath)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	if err := wal.Append(WALEntry{SessionID: 9, X: 1, Y: 1, TS: 10}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulate interrupted write by adding a short trailing fragment.
	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open wal append: %v", err)
	}
	if _, err := f.Write([]byte{1, 2, 3}); err != nil {
		t.Fatalf("write tail fragment: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close wal append: %v", err)
	}

	var got []WALEntry
	if err := ReplayWAL(walPath, 0, func(e WALEntry) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatalf("ReplayWAL: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != 9 {
		t.Fatalf("unexpected entries after partial tail: %+v", got)
	}
}
