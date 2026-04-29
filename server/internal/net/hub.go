package net

import (
	"sync"
)

const DefaultOutboundBuffer = 256

type Session struct {
	ID uint64
}

type Client struct {
	mu              sync.Mutex
	SessionID       uint64
	Outbound        chan []byte
	SlowConsumer    bool
	NeedResnapshot  bool
	subscribedChunk map[uint16]struct{}
}

type Hub struct {
	mu           sync.RWMutex
	sessions     map[uint64]Session
	clients      map[uint64]*Client
	chunkSubs    map[uint16]map[uint64]*Client
	outboundSize int
}

func NewHub(outboundSize int) *Hub {
	if outboundSize <= 0 {
		outboundSize = DefaultOutboundBuffer
	}
	return &Hub{
		sessions:     make(map[uint64]Session),
		clients:      make(map[uint64]*Client),
		chunkSubs:    make(map[uint16]map[uint64]*Client),
		outboundSize: outboundSize,
	}
}

func (h *Hub) RegisterSession(sess Session) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.clients[sess.ID]; ok {
		for chunkID := range existing.subscribedChunk {
			delete(h.chunkSubs[chunkID], sess.ID)
			if len(h.chunkSubs[chunkID]) == 0 {
				delete(h.chunkSubs, chunkID)
			}
		}
		close(existing.Outbound)
	}

	client := &Client{
		SessionID:       sess.ID,
		Outbound:        make(chan []byte, h.outboundSize),
		subscribedChunk: make(map[uint16]struct{}),
	}
	h.sessions[sess.ID] = sess
	h.clients[sess.ID] = client
	return client
}

func (h *Hub) RemoveSession(sessionID uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, ok := h.clients[sessionID]
	if !ok {
		return
	}
	for chunkID := range client.subscribedChunk {
		delete(h.chunkSubs[chunkID], sessionID)
		if len(h.chunkSubs[chunkID]) == 0 {
			delete(h.chunkSubs, chunkID)
		}
	}
	delete(h.sessions, sessionID)
	delete(h.clients, sessionID)
	close(client.Outbound)
}

func (h *Hub) Subscribe(sessionID uint64, chunkID uint16) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, ok := h.clients[sessionID]
	if !ok {
		return false
	}
	if _, ok := h.chunkSubs[chunkID]; !ok {
		h.chunkSubs[chunkID] = make(map[uint64]*Client)
	}
	h.chunkSubs[chunkID][sessionID] = client
	client.subscribedChunk[chunkID] = struct{}{}
	return true
}

func (h *Hub) Unsubscribe(sessionID uint64, chunkID uint16) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, ok := h.clients[sessionID]
	if !ok {
		return false
	}
	if subs, ok := h.chunkSubs[chunkID]; ok {
		delete(subs, sessionID)
		if len(subs) == 0 {
			delete(h.chunkSubs, chunkID)
		}
	}
	delete(client.subscribedChunk, chunkID)
	return true
}

func (h *Hub) Broadcast(chunkIDs []uint16, frame []byte) {
	seen := make(map[uint64]*Client)
	h.mu.RLock()
	for _, chunkID := range chunkIDs {
		for id, client := range h.chunkSubs[chunkID] {
			seen[id] = client
		}
	}
	h.mu.RUnlock()

	for _, client := range seen {
		client.enqueue(frame)
	}
}

// ClientCount returns the number of currently registered clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// BroadcastAll fans the frame out to every connected client regardless of
// chunk subscription state. Used for global frames such as Score updates.
func (h *Hub) BroadcastAll(frame []byte) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		c.enqueue(frame)
	}
}

func (c *Client) enqueue(frame []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case c.Outbound <- frame:
		return true
	default:
		c.SlowConsumer = true
		c.NeedResnapshot = true
		return false
	}
}

func (c *Client) SnapshotFlags() (slow bool, needResnapshot bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SlowConsumer, c.NeedResnapshot
}
