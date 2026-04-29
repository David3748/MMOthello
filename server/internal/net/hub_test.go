package net

import "testing"

type mockSender struct {
	payloads [][]byte
}

func (m *mockSender) Send(payload []byte) error {
	copied := append([]byte(nil), payload...)
	m.payloads = append(m.payloads, copied)
	return nil
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	h := NewHub(2)
	h.RegisterSession(Session{ID: 1})
	if ok := h.Subscribe(1, 10); !ok {
		t.Fatalf("Subscribe failed")
	}
	if ok := h.Unsubscribe(1, 10); !ok {
		t.Fatalf("Unsubscribe failed")
	}
}

func TestSlowConsumerFlagging(t *testing.T) {
	h := NewHub(1)
	slow := h.RegisterSession(Session{ID: 1})
	fast := h.RegisterSession(Session{ID: 2})
	h.Subscribe(1, 7)
	h.Subscribe(2, 7)

	// Fill slow client's queue.
	slow.Outbound <- []byte("existing")

	h.Broadcast([]uint16{7}, []byte("delta"))

	slowFlag, needsResnapshot := slow.SnapshotFlags()
	if !slowFlag || !needsResnapshot {
		t.Fatalf("slow consumer was not flagged")
	}
	fastFlag, _ := fast.SnapshotFlags()
	if fastFlag {
		t.Fatalf("fast consumer incorrectly flagged as slow")
	}
	select {
	case got := <-fast.Outbound:
		if string(got) != "delta" {
			t.Fatalf("unexpected fast payload: %q", string(got))
		}
	default:
		t.Fatalf("fast consumer did not receive message")
	}
}

func TestFlushOutboundUsesSenderInterface(t *testing.T) {
	h := NewHub(3)
	client := h.RegisterSession(Session{ID: 1})
	client.Outbound <- []byte("a")
	client.Outbound <- []byte("b")

	sender := &mockSender{}
	sent, err := FlushOutbound(client, sender, 0)
	if err != nil {
		t.Fatalf("FlushOutbound: %v", err)
	}
	if sent != 2 {
		t.Fatalf("sent mismatch: got=%d want=2", sent)
	}
	if len(sender.payloads) != 2 || string(sender.payloads[0]) != "a" || string(sender.payloads[1]) != "b" {
		t.Fatalf("unexpected payloads: %q", sender.payloads)
	}
}

