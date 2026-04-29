package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	OpHello       = byte(0x01)
	OpSubscribe   = byte(0x02)
	OpUnsubscribe = byte(0x03)
	OpPlace       = byte(0x04)
	OpPing        = byte(0x05)

	OpWelcome  = byte(0x81)
	OpSnapshot = byte(0x82)
	OpDelta    = byte(0x83)
	OpPlaceAck = byte(0x84)
	OpScore    = byte(0x85)
	OpPong     = byte(0x86)
	OpError    = byte(0x87)
)

const (
	TokenSize        = 32
	ChunkPackedBytes = 625
)

var (
	ErrFrameTooShort = errors.New("frame too short")
	ErrBadLength     = errors.New("frame length mismatch")
)

type Delta struct {
	X    uint16
	Y    uint16
	Cell uint8
}

type Frame interface {
	Opcode() byte
}

type Hello struct {
	Token [TokenSize]byte
}

func (Hello) Opcode() byte { return OpHello }

type Subscribe struct {
	ChunkID uint16
}

func (Subscribe) Opcode() byte { return OpSubscribe }

type Unsubscribe struct {
	ChunkID uint16
}

func (Unsubscribe) Opcode() byte { return OpUnsubscribe }

type Place struct {
	X uint16
	Y uint16
}

func (Place) Opcode() byte { return OpPlace }

type Ping struct {
	Nonce uint32
}

func (Ping) Opcode() byte { return OpPing }

type Welcome struct {
	SessionID    uint64
	Token        [TokenSize]byte
	Team         uint8
	ServerTimeMs int64
}

func (Welcome) Opcode() byte { return OpWelcome }

type Snapshot struct {
	ChunkID uint16
	Version uint64
	Packed  [ChunkPackedBytes]byte
}

func (Snapshot) Opcode() byte { return OpSnapshot }

type DeltaFrame struct {
	Entries []Delta
}

func (DeltaFrame) Opcode() byte { return OpDelta }

type PlaceAck struct {
	OK            uint8
	NextAllowedMs int64
	ErrCode       uint8
}

func (PlaceAck) Opcode() byte { return OpPlaceAck }

type Score struct {
	Black uint32
	White uint32
	Empty uint32
}

func (Score) Opcode() byte { return OpScore }

type Pong struct {
	Nonce        uint32
	ServerTimeMs int64
}

func (Pong) Opcode() byte { return OpPong }

type ErrorFrame struct {
	Code uint8
	Msg  string
}

func (ErrorFrame) Opcode() byte { return OpError }

// EncodeFrame returns the binary payload for a single websocket message.
// WebSocket already provides framing, so no length prefix is added.
func EncodeFrame(frame Frame) ([]byte, error) {
	return encodePayload(frame)
}

// DecodeFrame parses one websocket message payload.
func DecodeFrame(raw []byte) (Frame, error) {
	if len(raw) < 1 {
		return nil, ErrFrameTooShort
	}
	return decodePayload(raw)
}

func encodePayload(frame Frame) ([]byte, error) {
	switch v := frame.(type) {
	case Hello:
		out := make([]byte, 1+TokenSize)
		out[0] = OpHello
		copy(out[1:], v.Token[:])
		return out, nil
	case Subscribe:
		out := make([]byte, 3)
		out[0] = OpSubscribe
		binary.LittleEndian.PutUint16(out[1:], v.ChunkID)
		return out, nil
	case Unsubscribe:
		out := make([]byte, 3)
		out[0] = OpUnsubscribe
		binary.LittleEndian.PutUint16(out[1:], v.ChunkID)
		return out, nil
	case Place:
		out := make([]byte, 5)
		out[0] = OpPlace
		binary.LittleEndian.PutUint16(out[1:], v.X)
		binary.LittleEndian.PutUint16(out[3:], v.Y)
		return out, nil
	case Ping:
		out := make([]byte, 5)
		out[0] = OpPing
		binary.LittleEndian.PutUint32(out[1:], v.Nonce)
		return out, nil
	case Welcome:
		out := make([]byte, 1+8+TokenSize+1+8)
		out[0] = OpWelcome
		binary.LittleEndian.PutUint64(out[1:], v.SessionID)
		copy(out[9:], v.Token[:])
		out[41] = v.Team
		binary.LittleEndian.PutUint64(out[42:], uint64(v.ServerTimeMs))
		return out, nil
	case Snapshot:
		out := make([]byte, 1+2+8+ChunkPackedBytes)
		out[0] = OpSnapshot
		binary.LittleEndian.PutUint16(out[1:], v.ChunkID)
		binary.LittleEndian.PutUint64(out[3:], v.Version)
		copy(out[11:], v.Packed[:])
		return out, nil
	case DeltaFrame:
		out := make([]byte, 1+2+len(v.Entries)*5)
		out[0] = OpDelta
		binary.LittleEndian.PutUint16(out[1:], uint16(len(v.Entries)))
		offset := 3
		for _, d := range v.Entries {
			binary.LittleEndian.PutUint16(out[offset:], d.X)
			binary.LittleEndian.PutUint16(out[offset+2:], d.Y)
			out[offset+4] = d.Cell
			offset += 5
		}
		return out, nil
	case PlaceAck:
		out := make([]byte, 1+1+8+1)
		out[0] = OpPlaceAck
		out[1] = v.OK
		binary.LittleEndian.PutUint64(out[2:], uint64(v.NextAllowedMs))
		out[10] = v.ErrCode
		return out, nil
	case Score:
		out := make([]byte, 1+4+4+4)
		out[0] = OpScore
		binary.LittleEndian.PutUint32(out[1:], v.Black)
		binary.LittleEndian.PutUint32(out[5:], v.White)
		binary.LittleEndian.PutUint32(out[9:], v.Empty)
		return out, nil
	case Pong:
		out := make([]byte, 1+4+8)
		out[0] = OpPong
		binary.LittleEndian.PutUint32(out[1:], v.Nonce)
		binary.LittleEndian.PutUint64(out[5:], uint64(v.ServerTimeMs))
		return out, nil
	case ErrorFrame:
		msg := []byte(v.Msg)
		out := make([]byte, 1+1+2+len(msg))
		out[0] = OpError
		out[1] = v.Code
		binary.LittleEndian.PutUint16(out[2:], uint16(len(msg)))
		copy(out[4:], msg)
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported frame type %T", frame)
	}
}

func decodePayload(payload []byte) (Frame, error) {
	if len(payload) == 0 {
		return nil, ErrFrameTooShort
	}
	op := payload[0]
	switch op {
	case OpHello:
		if len(payload) != 1+TokenSize {
			return nil, ErrBadLength
		}
		var token [TokenSize]byte
		copy(token[:], payload[1:])
		return Hello{Token: token}, nil
	case OpSubscribe:
		if len(payload) != 3 {
			return nil, ErrBadLength
		}
		return Subscribe{ChunkID: binary.LittleEndian.Uint16(payload[1:])}, nil
	case OpUnsubscribe:
		if len(payload) != 3 {
			return nil, ErrBadLength
		}
		return Unsubscribe{ChunkID: binary.LittleEndian.Uint16(payload[1:])}, nil
	case OpPlace:
		if len(payload) != 5 {
			return nil, ErrBadLength
		}
		return Place{
			X: binary.LittleEndian.Uint16(payload[1:]),
			Y: binary.LittleEndian.Uint16(payload[3:]),
		}, nil
	case OpPing:
		if len(payload) != 5 {
			return nil, ErrBadLength
		}
		return Ping{Nonce: binary.LittleEndian.Uint32(payload[1:])}, nil
	case OpWelcome:
		if len(payload) != 50 {
			return nil, ErrBadLength
		}
		var token [TokenSize]byte
		copy(token[:], payload[9:41])
		return Welcome{
			SessionID:    binary.LittleEndian.Uint64(payload[1:]),
			Token:        token,
			Team:         payload[41],
			ServerTimeMs: int64(binary.LittleEndian.Uint64(payload[42:])),
		}, nil
	case OpSnapshot:
		if len(payload) != 636 {
			return nil, ErrBadLength
		}
		var packed [ChunkPackedBytes]byte
		copy(packed[:], payload[11:])
		return Snapshot{
			ChunkID: binary.LittleEndian.Uint16(payload[1:]),
			Version: binary.LittleEndian.Uint64(payload[3:11]),
			Packed:  packed,
		}, nil
	case OpDelta:
		if len(payload) < 3 {
			return nil, ErrBadLength
		}
		count := int(binary.LittleEndian.Uint16(payload[1:3]))
		if len(payload) != 3+count*5 {
			return nil, ErrBadLength
		}
		entries := make([]Delta, count)
		offset := 3
		for i := 0; i < count; i++ {
			entries[i] = Delta{
				X:    binary.LittleEndian.Uint16(payload[offset:]),
				Y:    binary.LittleEndian.Uint16(payload[offset+2:]),
				Cell: payload[offset+4],
			}
			offset += 5
		}
		return DeltaFrame{Entries: entries}, nil
	case OpPlaceAck:
		if len(payload) != 11 {
			return nil, ErrBadLength
		}
		return PlaceAck{
			OK:            payload[1],
			NextAllowedMs: int64(binary.LittleEndian.Uint64(payload[2:10])),
			ErrCode:       payload[10],
		}, nil
	case OpScore:
		if len(payload) != 13 {
			return nil, ErrBadLength
		}
		return Score{
			Black: binary.LittleEndian.Uint32(payload[1:5]),
			White: binary.LittleEndian.Uint32(payload[5:9]),
			Empty: binary.LittleEndian.Uint32(payload[9:13]),
		}, nil
	case OpPong:
		if len(payload) != 13 {
			return nil, ErrBadLength
		}
		return Pong{
			Nonce:        binary.LittleEndian.Uint32(payload[1:5]),
			ServerTimeMs: int64(binary.LittleEndian.Uint64(payload[5:13])),
		}, nil
	case OpError:
		if len(payload) < 4 {
			return nil, ErrBadLength
		}
		msgLen := int(binary.LittleEndian.Uint16(payload[2:4]))
		if len(payload) != 4+msgLen {
			return nil, ErrBadLength
		}
		return ErrorFrame{
			Code: payload[1],
			Msg:  string(payload[4:]),
		}, nil
	default:
		return nil, fmt.Errorf("unknown opcode 0x%02x", op)
	}
}
