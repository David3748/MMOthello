package net

// Sender abstracts transport writes (websocket, tcp, tests, etc).
type Sender interface {
	Send(payload []byte) error
}

// FlushOutbound sends up to max queued messages for a client.
// If max <= 0 it drains the full queue.
func FlushOutbound(client *Client, sender Sender, max int) (int, error) {
	sent := 0
	for {
		if max > 0 && sent >= max {
			return sent, nil
		}
		select {
		case payload := <-client.Outbound:
			if err := sender.Send(payload); err != nil {
				return sent, err
			}
			sent++
		default:
			return sent, nil
		}
	}
}
