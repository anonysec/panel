package knodepb

import (
	"encoding/binary"
	"fmt"
	"math"
)

// GenerateClientCertRequest is the request for generating an OpenVPN client certificate.
type GenerateClientCertRequest struct {
	Username string `protobuf:"bytes,1,opt,name=username,proto3" json:"username,omitempty"`
}

func (x *GenerateClientCertRequest) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *GenerateClientCertRequest) Reset()         { *x = GenerateClientCertRequest{} }
func (x *GenerateClientCertRequest) String() string { return x.Username }
func (x *GenerateClientCertRequest) ProtoMessage()  {}

// Marshal implements proto.Marshaler for wire-format encoding.
func (x *GenerateClientCertRequest) Marshal() ([]byte, error) {
	if x == nil {
		return nil, nil
	}
	return marshalString(1, x.Username), nil
}

// Unmarshal implements proto.Unmarshaler for wire-format decoding.
func (x *GenerateClientCertRequest) Unmarshal(b []byte) error {
	for len(b) > 0 {
		fieldNum, wireType, n := consumeTag(b)
		if n < 0 {
			return fmt.Errorf("invalid tag")
		}
		b = b[n:]
		if fieldNum == 1 && wireType == 2 {
			s, n := consumeString(b)
			if n < 0 {
				return fmt.Errorf("invalid string")
			}
			x.Username = s
			b = b[n:]
		} else {
			n := skipField(b, wireType)
			if n < 0 {
				return fmt.Errorf("cannot skip field")
			}
			b = b[n:]
		}
	}
	return nil
}

// GenerateClientCertResponse contains the generated cert, key, and CA.
type GenerateClientCertResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	CertPem string `protobuf:"bytes,2,opt,name=cert_pem,proto3" json:"cert_pem,omitempty"`
	KeyPem  string `protobuf:"bytes,3,opt,name=key_pem,proto3" json:"key_pem,omitempty"`
	CaPem   string `protobuf:"bytes,4,opt,name=ca_pem,proto3" json:"ca_pem,omitempty"`
	Message string `protobuf:"bytes,5,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *GenerateClientCertResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}
func (x *GenerateClientCertResponse) GetCertPem() string {
	if x != nil {
		return x.CertPem
	}
	return ""
}
func (x *GenerateClientCertResponse) GetKeyPem() string {
	if x != nil {
		return x.KeyPem
	}
	return ""
}
func (x *GenerateClientCertResponse) GetCaPem() string {
	if x != nil {
		return x.CaPem
	}
	return ""
}
func (x *GenerateClientCertResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *GenerateClientCertResponse) Reset()         { *x = GenerateClientCertResponse{} }
func (x *GenerateClientCertResponse) String() string { return x.Message }
func (x *GenerateClientCertResponse) ProtoMessage()  {}

// Marshal implements proto.Marshaler.
func (x *GenerateClientCertResponse) Marshal() ([]byte, error) {
	if x == nil {
		return nil, nil
	}
	var out []byte
	if x.Success {
		out = append(out, marshalVarint(1, 1)...)
	}
	out = append(out, marshalString(2, x.CertPem)...)
	out = append(out, marshalString(3, x.KeyPem)...)
	out = append(out, marshalString(4, x.CaPem)...)
	out = append(out, marshalString(5, x.Message)...)
	return out, nil
}

// Unmarshal implements proto.Unmarshaler.
func (x *GenerateClientCertResponse) Unmarshal(b []byte) error {
	for len(b) > 0 {
		fieldNum, wireType, n := consumeTag(b)
		if n < 0 {
			return fmt.Errorf("invalid tag")
		}
		b = b[n:]
		switch {
		case fieldNum == 1 && wireType == 0:
			v, n := consumeVarint(b)
			if n < 0 {
				return fmt.Errorf("invalid varint")
			}
			x.Success = v != 0
			b = b[n:]
		case fieldNum == 2 && wireType == 2:
			s, n := consumeString(b)
			if n < 0 {
				return fmt.Errorf("invalid string")
			}
			x.CertPem = s
			b = b[n:]
		case fieldNum == 3 && wireType == 2:
			s, n := consumeString(b)
			if n < 0 {
				return fmt.Errorf("invalid string")
			}
			x.KeyPem = s
			b = b[n:]
		case fieldNum == 4 && wireType == 2:
			s, n := consumeString(b)
			if n < 0 {
				return fmt.Errorf("invalid string")
			}
			x.CaPem = s
			b = b[n:]
		case fieldNum == 5 && wireType == 2:
			s, n := consumeString(b)
			if n < 0 {
				return fmt.Errorf("invalid string")
			}
			x.Message = s
			b = b[n:]
		default:
			n := skipField(b, wireType)
			if n < 0 {
				return fmt.Errorf("cannot skip field")
			}
			b = b[n:]
		}
	}
	return nil
}

// --- Minimal protobuf wire-format helpers ---

func marshalString(fieldNum uint64, s string) []byte {
	if s == "" {
		return nil
	}
	tag := encodeTag(fieldNum, 2) // wire type 2 = length-delimited
	lenBytes := encodeVarint(uint64(len(s)))
	out := make([]byte, 0, len(tag)+len(lenBytes)+len(s))
	out = append(out, tag...)
	out = append(out, lenBytes...)
	out = append(out, s...)
	return out
}

func marshalVarint(fieldNum uint64, v uint64) []byte {
	tag := encodeTag(fieldNum, 0) // wire type 0 = varint
	val := encodeVarint(v)
	out := make([]byte, 0, len(tag)+len(val))
	out = append(out, tag...)
	out = append(out, val...)
	return out
}

func encodeTag(fieldNum uint64, wireType uint64) []byte {
	return encodeVarint((fieldNum << 3) | wireType)
}

func encodeVarint(v uint64) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

func consumeTag(b []byte) (fieldNum uint64, wireType uint64, n int) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, 0, -1
	}
	return v >> 3, v & 0x7, n
}

func consumeVarint(b []byte) (uint64, int) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, -1
	}
	return v, n
}

func consumeString(b []byte) (string, int) {
	length, n := binary.Uvarint(b)
	if n <= 0 || length > math.MaxInt32 {
		return "", -1
	}
	total := n + int(length)
	if total > len(b) {
		return "", -1
	}
	return string(b[n:total]), total
}

func skipField(b []byte, wireType uint64) int {
	switch wireType {
	case 0: // varint
		_, n := binary.Uvarint(b)
		return n
	case 1: // 64-bit
		return 8
	case 2: // length-delimited
		length, n := binary.Uvarint(b)
		if n <= 0 {
			return -1
		}
		return n + int(length)
	case 5: // 32-bit
		return 4
	default:
		return -1
	}
}
