package channel

// Feishu WS Frame (pbbp2 protobuf, manually decoded)
//
// Proto schema (from @larksuiteoapi/node-sdk):
//   message Header { string key = 1; string value = 2; }
//   message Frame {
//     uint64 SeqID           = 1;
//     uint64 LogID           = 2;
//     int32  service         = 3;
//     int32  method          = 4;
//     repeated Header headers = 5;
//     string payloadEncoding = 6;
//     string payloadType     = 7;
//     bytes  payload         = 8;
//     string LogIDNew        = 9;
//   }

import (
	"encoding/binary"
	"fmt"
)

type feishuFrame struct {
	SeqID           uint64
	LogID           uint64
	Service         int32
	Method          int32 // 1=data, 2=control, 3=ping, 4=pong
	Headers         []feishuFrameHeader
	PayloadEncoding string
	PayloadType     string
	Payload         []byte
}

type feishuFrameHeader struct {
	Key   string
	Value string
}

// FrameMethod values (from SDK FrameType enum)
const (
	feishuFrameMethodData    = 1
	feishuFrameMethodControl = 2
	feishuFrameMethodPing    = 3
	feishuFrameMethodPong    = 4
)

// parseFeishuFrame decodes a protobuf-encoded pbbp2.Frame.
func parseFeishuFrame(data []byte) (*feishuFrame, error) {
	frame := &feishuFrame{}
	i := 0
	for i < len(data) {
		if i >= len(data) {
			break
		}
		// Read field tag (varint)
		tag, n := decodeVarint(data[i:])
		if n <= 0 {
			return nil, fmt.Errorf("invalid varint at %d", i)
		}
		i += n

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // SeqID uint64
			v, n := decodeVarint(data[i:])
			if n <= 0 {
				return nil, fmt.Errorf("field 1 varint error")
			}
			frame.SeqID = v
			i += n
		case 2: // LogID uint64
			v, n := decodeVarint(data[i:])
			if n <= 0 {
				return nil, fmt.Errorf("field 2 varint error")
			}
			frame.LogID = v
			i += n
		case 3: // service int32
			v, n := decodeVarint(data[i:])
			if n <= 0 {
				return nil, fmt.Errorf("field 3 varint error")
			}
			frame.Service = int32(v)
			i += n
		case 4: // method int32
			v, n := decodeVarint(data[i:])
			if n <= 0 {
				return nil, fmt.Errorf("field 4 varint error")
			}
			frame.Method = int32(v)
			i += n
		case 5: // headers (repeated Header, length-delimited)
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			length, n := decodeVarint(data[i:])
			if n <= 0 {
				return nil, fmt.Errorf("field 5 length error")
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return nil, fmt.Errorf("field 5 out of bounds")
			}
			hdr, err := parseFeishuFrameHeader(data[i:end])
			if err == nil {
				frame.Headers = append(frame.Headers, hdr)
			}
			i = end
		case 6: // payloadEncoding string
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			s, n, err := decodeString(data[i:])
			if err != nil {
				return nil, err
			}
			frame.PayloadEncoding = s
			i += n
		case 7: // payloadType string
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			s, n, err := decodeString(data[i:])
			if err != nil {
				return nil, err
			}
			frame.PayloadType = s
			i += n
		case 8: // payload bytes
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			b, n, err := decodeBytes(data[i:])
			if err != nil {
				return nil, err
			}
			frame.Payload = b
			i += n
		default:
			i = skipField(data, i, wireType)
		}
	}
	return frame, nil
}

func parseFeishuFrameHeader(data []byte) (feishuFrameHeader, error) {
	hdr := feishuFrameHeader{}
	i := 0
	for i < len(data) {
		tag, n := decodeVarint(data[i:])
		if n <= 0 {
			break
		}
		i += n
		fieldNum := tag >> 3
		wireType := tag & 0x7
		switch fieldNum {
		case 1:
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			s, n2, err := decodeString(data[i:])
			if err != nil {
				return hdr, err
			}
			hdr.Key = s
			i += n2
		case 2:
			if wireType != 2 {
				i = skipField(data, i, wireType)
				continue
			}
			s, n2, err := decodeString(data[i:])
			if err != nil {
				return hdr, err
			}
			hdr.Value = s
			i += n2
		default:
			i = skipField(data, i, wireType)
		}
	}
	return hdr, nil
}

// getHeader returns the value for the given key, or "".
func (f *feishuFrame) getHeader(key string) string {
	for _, h := range f.Headers {
		if h.Key == key {
			return h.Value
		}
	}
	return ""
}

// encodeFeishuPong encodes a pong frame as protobuf bytes.
func encodeFeishuPong(seqID uint64) []byte {
	var b []byte
	// field 1 (SeqID) varint
	b = appendVarintField(b, 1, seqID)
	// field 4 (method) varint = 4 (pong)
	b = appendVarintField(b, 4, feishuFrameMethodPong)
	return b
}

// ── protobuf helpers ──────────────────────────────────────────────────────────

func decodeVarint(b []byte) (uint64, int) {
	var x uint64
	var s uint
	for i, c := range b {
		if c < 0x80 {
			return x | uint64(c)<<s, i + 1
		}
		x |= uint64(c&0x7f) << s
		s += 7
		if s >= 64 {
			return 0, -1
		}
	}
	return 0, -1
}

func decodeString(b []byte) (string, int, error) {
	bs, n, err := decodeBytes(b)
	return string(bs), n, err
}

func decodeBytes(b []byte) ([]byte, int, error) {
	length, n := decodeVarint(b)
	if n <= 0 {
		return nil, 0, fmt.Errorf("bytes length varint error")
	}
	end := n + int(length)
	if end > len(b) {
		return nil, 0, fmt.Errorf("bytes out of bounds: need %d have %d", end, len(b))
	}
	return b[n:end], end, nil
}

func skipField(data []byte, i int, wireType uint64) int {
	switch wireType {
	case 0: // varint
		_, n := decodeVarint(data[i:])
		if n > 0 {
			return i + n
		}
	case 1: // 64-bit
		return i + 8
	case 2: // length-delimited
		length, n := decodeVarint(data[i:])
		if n > 0 {
			return i + n + int(length)
		}
	case 5: // 32-bit
		return i + 4
	}
	return i + 1
}

func appendVarintField(b []byte, fieldNum uint64, v uint64) []byte {
	tag := (fieldNum << 3) | 0 // wireType 0 = varint
	b = appendVarint(b, tag)
	b = appendVarint(b, v)
	return b
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// encodePingFrame builds a protobuf ping frame for keepalive.
func encodePingFrame(seqID uint64, serviceID string) []byte {
	var b []byte
	b = appendVarintField(b, 1, seqID)
	b = appendVarintField(b, 4, feishuFrameMethodPing)
	// header: type=ping, service_id=xxx
	if serviceID != "" {
		b = appendHeaderField(b, 5, "service-id", serviceID)
	}
	return b
}

func appendHeaderField(b []byte, fieldNum uint64, key, value string) []byte {
	// encode Header message inline
	inner := appendStringField(nil, 1, key)
	inner = appendStringField(inner, 2, value)
	// write as length-delimited field
	tag := (fieldNum << 3) | 2
	b = appendVarint(b, tag)
	b = appendVarint(b, uint64(len(inner)))
	b = append(b, inner...)
	return b
}

func appendStringField(b []byte, fieldNum uint64, s string) []byte {
	tag := (fieldNum << 3) | 2
	b = appendVarint(b, tag)
	b = appendVarint(b, uint64(len(s)))
	b = append(b, s...)
	return b
}

// suppress unused import
var _ = binary.LittleEndian
