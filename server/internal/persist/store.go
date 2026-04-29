package persist

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrNoSnapshot = errors.New("no snapshot found")

type SnapshotMeta struct {
	TimestampUnix int64  `json:"timestamp_unix"`
	TimestampMs   int64  `json:"timestamp_ms"`
	SnapshotFile  string `json:"snapshot_file"`
}

type Snapshot struct {
	Data []byte
	Meta SnapshotMeta
}

type WALEntry struct {
	SessionID uint64
	X         uint16
	Y         uint16
	Team      uint8
	TS        int64
}

type WAL struct {
	mu sync.Mutex
	f  *os.File
	bw *bufio.Writer
}

func WriteSnapshot(dir string, ts time.Time, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	snapName := fmt.Sprintf("snapshot-%d.bin", ts.Unix())
	snapPath := filepath.Join(dir, snapName)
	snapTmp := snapPath + ".tmp"

	if err := os.WriteFile(snapTmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(snapTmp, snapPath); err != nil {
		return err
	}

	meta := SnapshotMeta{
		TimestampUnix: ts.Unix(),
		TimestampMs:   ts.UnixMilli(),
		SnapshotFile:  snapName,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	metaPath := filepath.Join(dir, "meta.json")
	metaTmp := metaPath + ".tmp"
	if err := os.WriteFile(metaTmp, metaBytes, 0o644); err != nil {
		return err
	}
	return os.Rename(metaTmp, metaPath)
}

func LoadLatestSnapshot(dir string) (Snapshot, error) {
	metaPath := filepath.Join(dir, "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, ErrNoSnapshot
		}
		return Snapshot{}, err
	}

	var meta SnapshotMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return Snapshot{}, err
	}
	if meta.TimestampMs == 0 && meta.TimestampUnix != 0 {
		meta.TimestampMs = meta.TimestampUnix * 1000
	}

	dataPath := filepath.Join(dir, meta.SnapshotFile)
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Data: data, Meta: meta}, nil
}

func OpenWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &WAL{
		f:  f,
		bw: bufio.NewWriterSize(f, 4096),
	}, nil
}

func (w *WAL) Append(entry WALEntry) error {
	var buf [21]byte
	binary.LittleEndian.PutUint64(buf[0:], entry.SessionID)
	binary.LittleEndian.PutUint16(buf[8:], entry.X)
	binary.LittleEndian.PutUint16(buf[10:], entry.Y)
	buf[12] = entry.Team
	binary.LittleEndian.PutUint64(buf[13:], uint64(entry.TS))
	w.mu.Lock()
	_, err := w.bw.Write(buf[:])
	w.mu.Unlock()
	return err
}

func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.bw.Flush(); err != nil {
		return err
	}
	return w.f.Sync()
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.bw.Flush(); err != nil {
		_ = w.f.Close()
		return err
	}
	return w.f.Close()
}

func (w *WAL) CompactAfter(cutoffMs int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.bw.Flush(); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	raw, err := io.ReadAll(w.f)
	if err != nil {
		return err
	}

	kept := make([]byte, 0, len(raw))
	for offset := 0; offset+21 <= len(raw); offset += 21 {
		ts := int64(binary.LittleEndian.Uint64(raw[offset+13 : offset+21]))
		if ts > cutoffMs {
			kept = append(kept, raw[offset:offset+21]...)
		}
	}

	if err := w.f.Truncate(0); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.f.Write(kept); err != nil {
		return err
	}
	return w.f.Sync()
}

func ReplayWAL(path string, sinceMs int64, fn func(WALEntry) error) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	var buf [21]byte
	for {
		_, err := io.ReadFull(f, buf[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				// Ignore a trailing partial write.
				return nil
			}
			return err
		}

		e := WALEntry{
			SessionID: binary.LittleEndian.Uint64(buf[0:8]),
			X:         binary.LittleEndian.Uint16(buf[8:10]),
			Y:         binary.LittleEndian.Uint16(buf[10:12]),
			Team:      buf[12],
			TS:        int64(binary.LittleEndian.Uint64(buf[13:21])),
		}
		if e.TS <= sinceMs {
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
}
