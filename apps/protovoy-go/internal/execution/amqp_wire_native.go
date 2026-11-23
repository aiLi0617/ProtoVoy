//go:build !js

package execution

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

const amqpFrameLimit = 128 << 10

// amqpBoundedConn validates broker-controlled allocation lengths before
// handing frames to the AMQP decoder, including nested tables.
type amqpBoundedConn struct {
	net.Conn
	remaining int
	bodyLeft  uint64
	buffer    []byte
}

func (conn *amqpBoundedConn) Read(out []byte) (int, error) {
	if len(out) == 0 {
		return 0, nil
	}
	if len(conn.buffer) == 0 {
		var header [7]byte
		if _, err := io.ReadFull(conn.Conn, header[:]); err != nil {
			return 0, err
		}
		size := binary.BigEndian.Uint32(header[3:])
		if size > amqpFrameLimit-8 || conn.remaining < int(size)+8 || binary.BigEndian.Uint16(header[1:3]) > 1 {
			return 0, errors.New("AMQP frame exceeds its size or channel limit")
		}
		frame := make([]byte, int(size)+8)
		copy(frame, header[:])
		if _, err := io.ReadFull(conn.Conn, frame[7:]); err != nil {
			return 0, err
		}
		if frame[len(frame)-1] != 0xce || !conn.validFrame(header[0], frame[7:len(frame)-1]) {
			return 0, errors.New("invalid or oversized AMQP frame")
		}
		conn.remaining -= len(frame)
		conn.buffer = frame
	}
	n := copy(out, conn.buffer)
	conn.buffer = conn.buffer[n:]
	return n, nil
}

func (conn *amqpBoundedConn) validFrame(kind byte, payload []byte) bool {
	switch kind {
	case 1:
		return validAMQPMethod(payload)
	case 2:
		if len(payload) < 14 || conn.bodyLeft != 0 {
			return false
		}
		conn.bodyLeft = binary.BigEndian.Uint64(payload[4:12])
		if conn.bodyLeft > amqpBodyLimit {
			return false
		}
		flags := binary.BigEndian.Uint16(payload[12:14])
		if flags&0x2000 == 0 {
			return true
		}
		fields := amqpFields(payload[14:])
		for _, flag := range []uint16{0x8000, 0x4000} {
			if flags&flag != 0 && !fields.short() {
				return false
			}
		}
		return fields.container(0, true)
	case 3:
		if len(payload) == 0 || uint64(len(payload)) > conn.bodyLeft {
			return false
		}
		conn.bodyLeft -= uint64(len(payload))
		return true
	case 8:
		return len(payload) == 0
	default:
		return false
	}
}

func validAMQPMethod(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}
	class, method := binary.BigEndian.Uint16(payload), binary.BigEndian.Uint16(payload[2:])
	fields := amqpFields(payload[4:])
	switch [2]uint16{class, method} {
	case [2]uint16{10, 10}: // connection.start
		return fields.skip(2) && fields.container(0, true) && fields.long() && fields.long()
	case [2]uint16{20, 11}: // channel.open-ok
		return fields.long()
	case [2]uint16{60, 80}, [2]uint16{60, 120}: // Only one publish is outstanding.
		return len(fields) == 9 && binary.BigEndian.Uint64(fields) == 1
	// Remaining broker methods contain only fixed-width values and short strings.
	case [2]uint16{10, 30}, [2]uint16{10, 41}, [2]uint16{10, 50}, [2]uint16{10, 51},
		[2]uint16{10, 60}, [2]uint16{10, 61}, [2]uint16{20, 20}, [2]uint16{20, 21},
		[2]uint16{20, 40}, [2]uint16{20, 41}, [2]uint16{40, 11}, [2]uint16{50, 11},
		[2]uint16{50, 21}, [2]uint16{60, 11}, [2]uint16{60, 21}, [2]uint16{60, 30},
		[2]uint16{60, 31}, [2]uint16{60, 50}, [2]uint16{60, 60}, [2]uint16{85, 11}:
		return true
	default:
		return false
	}
}

type amqpFields []byte

func (fields *amqpFields) skip(n int) bool {
	if len(*fields) < n {
		return false
	}
	*fields = (*fields)[n:]
	return true
}

func (fields *amqpFields) short() bool {
	return len(*fields) > 0 && fields.skip(1+int((*fields)[0]))
}

func (fields *amqpFields) blob() (amqpFields, bool) {
	if len(*fields) < 4 {
		return nil, false
	}
	length := binary.BigEndian.Uint32(*fields)
	if length > uint32(len(*fields)-4) {
		return nil, false
	}
	value := (*fields)[4 : 4+int(length)]
	*fields = (*fields)[4+int(length):]
	return value, true
}

func (fields *amqpFields) long() bool {
	_, ok := fields.blob()
	return ok
}

func (fields *amqpFields) container(depth int, table bool) bool {
	nested, ok := fields.blob()
	if !ok || depth >= 16 {
		return false
	}
	for len(nested) > 0 {
		if table && !nested.short() || len(nested) == 0 {
			return false
		}
		kind := nested[0]
		nested = nested[1:]
		switch kind {
		case 'F', 'A':
			ok = nested.container(depth+1, kind == 'F')
		case 'S', 'x':
			ok = nested.long()
		case 't', 'B', 'b':
			ok = nested.skip(1)
		case 's', 'u':
			ok = nested.skip(2)
		case 'I', 'i', 'f':
			ok = nested.skip(4)
		case 'D':
			ok = nested.skip(5)
		case 'l', 'd', 'T':
			ok = nested.skip(8)
		case 'V':
			ok = true
		default:
			ok = false
		}
		if !ok {
			return false
		}
	}
	return true
}
