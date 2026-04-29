package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"mmothello/server/internal/auth"
	"mmothello/server/internal/board"
	"mmothello/server/internal/game"
	netpkg "mmothello/server/internal/net"
	"mmothello/server/internal/persist"
	"mmothello/server/internal/protocol"
	"mmothello/server/internal/ratelimit"
	"mmothello/server/internal/wsframe"
)

func main() {
	addr := envOrDefault("MMOTHELLO_ADDR", ":8080")
	dataDir := envOrDefault("MMOTHELLO_DATA", "./data")

	b := board.NewBoard()
	b.Seed()
	g := game.New(b)
	hub := netpkg.NewHub(netpkg.DefaultOutboundBuffer)
	tokens := auth.NewTokenManager(30 * 24 * time.Hour)
	var nextSessionID atomic.Uint64

	// Restore from disk if available.
	if err := restoreFromDisk(b, dataDir); err != nil {
		log.Printf("restore skipped: %v", err)
	}

	// Open WAL for new placements.
	walPath := filepath.Join(dataDir, "wal.log")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}
	wal, err := persist.OpenWAL(walPath)
	if err != nil {
		log.Fatalf("open wal: %v", err)
	}
	defer wal.Close()

	// Background goroutines.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go walSyncer(ctx, wal)
	go snapshotWriter(ctx, b, dataDir)
	go scoreBroadcaster(ctx, hub, b)

	teamPicker := newTeamPicker()

	// Anti-abuse: per-IP place rate (matches per-session 2 s cooldown) and
	// per-IP simultaneous WS connection cap. Env vars override for loadtests.
	placeLimiter := ratelimit.New(envFloat("MMOTHELLO_PLACE_RATE", 0.5),
		envFloat("MMOTHELLO_PLACE_BURST", 1.0), 10*time.Minute)
	connCap := ratelimit.NewConnCap(envInt("MMOTHELLO_CONN_CAP", 5))
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				placeLimiter.SweepIdle()
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		token, sessionID, err := issueOrReuseToken(tokens, &nextSessionID, r, now)
		if err != nil {
			http.Error(w, "failed to issue session", http.StatusInternalServerError)
			return
		}
		writeSessionCookie(w, token)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w,
			`{"sessionID":%d,"team":%d,"cooldownMs":%d}`+"\n",
			sessionID, teamPicker.assign(sessionID), game.Cooldown.Milliseconds(),
		)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if err := serveWS(w, r, tokens, hub, g, b, teamPicker, wal, placeLimiter, connCap); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		bk, wt, em := b.Score()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w,
			`{"black":%d,"white":%d,"empty":%d,"clients":%d}`+"\n",
			bk, wt, em, hub.ClientCount(),
		)
	})
	mux.Handle("/", http.FileServer(http.Dir("./client/dist")))

	srv := &http.Server{
		Addr:              addr,
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("mmothello listening addr=%s data=%s board=%dx%d cooldown=%s",
		addr, dataDir, board.BoardSize, board.BoardSize, game.Cooldown.Round(time.Second))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

// teamPicker assigns teams to sessions; sticky once chosen so reconnects
// return to the same color.
type teamPicker struct {
	mu      sync.Mutex
	by      map[uint64]uint8
	black   uint64
	white   uint64
}

func newTeamPicker() *teamPicker {
	return &teamPicker{by: make(map[uint64]uint8)}
}

func (t *teamPicker) assign(sessionID uint64) uint8 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v, ok := t.by[sessionID]; ok {
		return v
	}
	var v uint8 = 1
	if t.white < t.black {
		v = 2
	} else if t.black < t.white {
		v = 1
	} else {
		// Tied: deterministic by parity.
		if sessionID%2 == 0 {
			v = 2
		} else {
			v = 1
		}
	}
	t.by[sessionID] = v
	if v == 1 {
		t.black++
	} else {
		t.white++
	}
	return v
}

// ---------- background loops ----------

func walSyncer(ctx context.Context, wal *persist.WAL) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = wal.Sync()
			return
		case <-ticker.C:
			if err := wal.Sync(); err != nil {
				log.Printf("wal sync: %v", err)
			}
		}
	}
}

func snapshotWriter(ctx context.Context, b *board.Board, dir string) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.LockAllRead()
			data := append([]byte(nil), encodeBoard(b)...)
			b.UnlockAllRead()
			if err := persist.WriteSnapshot(dir, time.Now(), data); err != nil {
				log.Printf("snapshot: %v", err)
			}
		}
	}
}

// encodeBoard returns the full packed board (250000 bytes). Caller must hold
// at least RLock on all chunks for consistency.
func encodeBoard(b *board.Board) []byte {
	out := make([]byte, board.PackedByteSize)
	for cy := 0; cy < board.ChunksPerAxis; cy++ {
		for cx := 0; cx < board.ChunksPerAxis; cx++ {
			x0 := cx * board.ChunkSize
			y0 := cy * board.ChunkSize
			for ly := 0; ly < board.ChunkSize; ly++ {
				for lx := 0; lx < board.ChunkSize; lx++ {
					i := (y0+ly)*board.BoardSize + (x0 + lx)
					byteIdx := i / 4
					shift := uint((3 - (i % 4)) * 2)
					out[byteIdx] |= byte(b.Get(x0+lx, y0+ly)&0x03) << shift
				}
			}
		}
	}
	return out
}

func decodeBoardInto(b *board.Board, packed []byte) {
	if len(packed) != board.PackedByteSize {
		log.Printf("decodeBoard: bad size %d", len(packed))
		return
	}
	for y := 0; y < board.BoardSize; y++ {
		for x := 0; x < board.BoardSize; x++ {
			i := y*board.BoardSize + x
			shift := uint((3 - (i % 4)) * 2)
			c := board.Cell((packed[i/4] >> shift) & 0x03)
			b.Set(x, y, c)
		}
	}
}

func restoreFromDisk(b *board.Board, dir string) error {
	snap, err := persist.LoadLatestSnapshot(dir)
	if err != nil {
		return err
	}
	decodeBoardInto(b, snap.Data)
	// Recount per-chunk after restore.
	b.RecountChunks()
	walPath := filepath.Join(dir, "wal.log")
	return persist.ReplayWAL(walPath, snap.Meta.TimestampUnix, func(e persist.WALEntry) error {
		// We don't replay placement logic here (the snapshot was taken after
		// the placement was applied in memory). The WAL is only for bridging
		// the window between snapshot writes; replay is best-effort logging.
		return nil
	})
}

func scoreBroadcaster(ctx context.Context, hub *netpkg.Hub, b *board.Board) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var black, white uint32
			for id := uint16(0); id < board.TotalChunks; id++ {
				bk, wt := b.ChunkCounts(id)
				black += bk
				white += wt
			}
			empty := uint32(board.CellsPerBoard) - black - white
			payload, err := protocol.EncodeFrame(protocol.Score{Black: black, White: white, Empty: empty})
			if err != nil {
				continue
			}
			hub.BroadcastAll(payload)
		}
	}
}

// ---------- WS handling ----------

func serveWS(
	w http.ResponseWriter, r *http.Request,
	tokens *auth.TokenManager, hub *netpkg.Hub, g *game.Game, b *board.Board,
	teams *teamPicker, wal *persist.WAL,
	placeLimiter *ratelimit.TokenBucket, connCap *ratelimit.ConnCap,
) error {
	token, ok := readTokenCookie(r)
	if !ok {
		return errors.New("missing or invalid session cookie")
	}
	sessionID, valid := tokens.Validate(token)
	if !valid {
		return errors.New("session token not recognized")
	}
	if !isWebSocketUpgrade(r) {
		return errors.New("websocket upgrade required")
	}
	clientIP := remoteIP(r)
	if !connCap.Acquire(clientIP) {
		return errors.New("too many connections from this IP")
	}
	defer connCap.Release(clientIP)
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return errors.New("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("server does not support hijacking")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return fmt.Errorf("hijack failed: %w", err)
	}

	accept := websocketAccept(key)
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := io.WriteString(rw, resp); err != nil {
		_ = conn.Close()
		return err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return err
	}

	team := teams.assign(sessionID)
	teamCell := board.CellBlack
	if team == 2 {
		teamCell = board.CellWhite
	}

	client := hub.RegisterSession(netpkg.Session{ID: sessionID})
	gsess := &game.Session{ID: sessionID, Team: teamCell}

	// Serialize all writes to the underlying conn — multiple goroutines can
	// produce frames (writer drain + WS-level pong on reader thread).
	var writeMu sync.Mutex
	writeFrame := func(op byte, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return wsframe.Write(conn, op, payload)
	}

	writerDone := make(chan struct{})
	var writerOnce sync.Once
	closeAll := func() {
		writerOnce.Do(func() {
			hub.RemoveSession(sessionID)
			_ = conn.Close()
		})
	}
	go func() {
		defer close(writerDone)
		for payload := range client.Outbound {
			if err := writeFrame(wsframe.OpBinary, payload); err != nil {
				return
			}
		}
	}()

	enqueue := func(payload []byte) {
		select {
		case client.Outbound <- payload:
		default:
			// Slow consumer; drop. Hub-side broadcasts already mark the client
			// as needing a re-snapshot on its next subscribe.
		}
	}

	// Send Welcome.
	{
		var tok [protocol.TokenSize]byte
		copy(tok[:], token[:])
		w := protocol.Welcome{
			SessionID:    sessionID,
			Token:        tok,
			Team:         team,
			ServerTimeMs: time.Now().UnixMilli(),
		}
		if pl, err := protocol.EncodeFrame(w); err == nil {
			enqueue(pl)
		}
	}

	defer func() {
		closeAll()
		<-writerDone
	}()

	// Reader loop. Read through the buffered reader returned by Hijack so we
	// don't lose any bytes already buffered after the handshake response.
	for {
		f, err := wsframe.Read(rw.Reader)
		if err != nil {
			return nil
		}
		switch f.Opcode {
		case wsframe.OpClose:
			return nil
		case wsframe.OpPing:
			_ = writeFrame(wsframe.OpPong, f.Payload)
			continue
		case wsframe.OpPong:
			continue
		case wsframe.OpBinary, wsframe.OpText:
			// fall through
		default:
			continue
		}

		frame, err := protocol.DecodeFrame(f.Payload)
		if err != nil {
			continue
		}
		switch v := frame.(type) {
		case protocol.Hello:
			// Session is already authenticated via cookie; ignore the token here.
			_ = v
		case protocol.Subscribe:
			if v.ChunkID >= board.TotalChunks {
				continue
			}
			if !hub.Subscribe(sessionID, v.ChunkID) {
				continue
			}
			b.LockChunkRead(v.ChunkID)
			var packed [protocol.ChunkPackedBytes]byte
			b.PackChunk(v.ChunkID, packed[:])
			version := b.ChunkVersion(v.ChunkID)
			b.UnlockChunkRead(v.ChunkID)
			snap := protocol.Snapshot{ChunkID: v.ChunkID, Version: version, Packed: packed}
			if pl, err := protocol.EncodeFrame(snap); err == nil {
				enqueue(pl)
			}
		case protocol.Unsubscribe:
			hub.Unsubscribe(sessionID, v.ChunkID)
		case protocol.Ping:
			pong := protocol.Pong{Nonce: v.Nonce, ServerTimeMs: time.Now().UnixMilli()}
			if pl, err := protocol.EncodeFrame(pong); err == nil {
				enqueue(pl)
			}
		case protocol.Place:
			if !placeLimiter.Allow(clientIP) {
				ack := protocol.PlaceAck{
					OK:            0,
					NextAllowedMs: time.Now().UnixMilli() + game.Cooldown.Milliseconds(),
					ErrCode:       game.ErrCodeRateLimit,
				}
				if pl, err := protocol.EncodeFrame(ack); err == nil {
					enqueue(pl)
				}
				continue
			}
			handlePlace(enqueue, hub, g, gsess, wal, int(v.X), int(v.Y))
		}
	}
}

func handlePlace(enqueue func([]byte), hub *netpkg.Hub, g *game.Game, gsess *game.Session, wal *persist.WAL, x, y int) {
	flips, err := g.Place(gsess, x, y)
	if err != nil {
		ack := protocol.PlaceAck{
			OK:            0,
			NextAllowedMs: gsess.LastPlaceUnix + game.Cooldown.Milliseconds(),
			ErrCode:       game.ErrorCode(err),
		}
		if pl, e := protocol.EncodeFrame(ack); e == nil {
			enqueue(pl)
		}
		return
	}
	now := time.Now().UnixMilli()
	_ = wal.Append(persist.WALEntry{SessionID: gsess.ID, X: uint16(x), Y: uint16(y), TS: now / 1000})

	entries := make([]protocol.Delta, 0, len(flips)+1)
	entries = append(entries, protocol.Delta{X: uint16(x), Y: uint16(y), Cell: uint8(gsess.Team)})
	for _, f := range flips {
		entries = append(entries, protocol.Delta{X: f.X, Y: f.Y, Cell: uint8(gsess.Team)})
	}
	if pl, err := protocol.EncodeFrame(protocol.DeltaFrame{Entries: entries}); err == nil {
		seen := map[uint16]struct{}{board.ChunkOf(x, y): {}}
		for _, c := range flips {
			seen[board.ChunkOf(int(c.X), int(c.Y))] = struct{}{}
		}
		ids := make([]uint16, 0, len(seen))
		for id := range seen {
			ids = append(ids, id)
		}
		hub.Broadcast(ids, pl)
	}
	ack := protocol.PlaceAck{
		OK:            1,
		NextAllowedMs: now + game.Cooldown.Milliseconds(),
		ErrCode:       0,
	}
	if pl, err := protocol.EncodeFrame(ack); err == nil {
		enqueue(pl)
	}
}

// ---------- helpers (kept compatible with prior version) ----------

func issueOrReuseToken(
	tokens *auth.TokenManager,
	nextSessionID *atomic.Uint64,
	r *http.Request,
	now time.Time,
) ([auth.TokenSize]byte, uint64, error) {
	token, ok := readTokenCookie(r)
	if ok {
		if rotated, didRotate, sessionID, err := tokens.RotateIfExpired(token, now); err == nil {
			if didRotate {
				return rotated, sessionID, nil
			}
			return token, sessionID, nil
		}
	}
	sessionID := nextSessionID.Add(1)
	newToken, err := tokens.Generate(sessionID, now)
	return newToken, sessionID, err
}

func writeSessionCookie(w http.ResponseWriter, token [auth.TokenSize]byte) {
	http.SetCookie(w, &http.Cookie{
		Name:     "mmothello_token",
		Value:    hex.EncodeToString(token[:]),
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func readTokenCookie(r *http.Request) ([auth.TokenSize]byte, bool) {
	if c, err := r.Cookie("mmothello_token"); err == nil {
		raw, err := hex.DecodeString(c.Value)
		if err == nil && len(raw) == auth.TokenSize {
			var token [auth.TokenSize]byte
			copy(token[:], raw)
			return token, true
		}
	}
	// Fallback: token in query string (used by bots/loadtests).
	if v := r.URL.Query().Get("token"); v != "" {
		raw, err := hex.DecodeString(v)
		if err == nil && len(raw) == auth.TokenSize {
			var token [auth.TokenSize]byte
			copy(token[:], raw)
			return token, true
		}
	}
	return [auth.TokenSize]byte{}, false
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func websocketAccept(key string) string {
	const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// remoteIP returns the client's IP, honoring X-Forwarded-For when present
// (first hop only). Used as the per-IP key for rate limiting.
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}

func envInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(name string, fallback float64) float64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s from=%s dur_ms=%d", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start).Milliseconds())
	})
}

