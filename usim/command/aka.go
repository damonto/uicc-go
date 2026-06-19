package command

import (
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/apdu"
)

type Authenticate3G struct {
	Rand []byte
	AUTN []byte
}

type Authenticate3GResult struct {
	RES    []byte
	CK     []byte
	IK     []byte
	AUTS   []byte
	Reject bool
}

func (r Authenticate3GResult) IsSuccess() bool {
	return len(r.RES) != 0 && len(r.CK) != 0 && len(r.IK) != 0
}

func (r Authenticate3GResult) IsSynchronizationFailure() bool {
	return len(r.AUTS) != 0
}

func (r Authenticate3GResult) IsAuthenticationReject() bool {
	return r.Reject
}

func (c Authenticate3G) MarshalBinary() ([]byte, error) {
	data := make([]byte, 0, len(c.Rand)+len(c.AUTN)+2)
	data = append(data, byte(len(c.Rand)))
	data = append(data, c.Rand...)
	data = append(data, byte(len(c.AUTN)))
	data = append(data, c.AUTN...)

	return apdu.Request{
		CLA:  0x00,
		INS:  0x88,
		P1:   0x00,
		P2:   0x81,
		Data: data,
	}.MarshalBinary()
}

func (r Authenticate3GResult) MarshalBinary() ([]byte, error) {
	switch {
	case len(r.RES) != 0 || len(r.CK) != 0 || len(r.IK) != 0:
		if !r.IsSuccess() {
			return nil, errors.New("marshaling AKA response: incomplete success result")
		}
		out := []byte{0xDB}
		var err error
		out, err = appendAKAChunk(out, "RES", r.RES)
		if err != nil {
			return nil, err
		}
		out, err = appendAKAChunk(out, "CK", r.CK)
		if err != nil {
			return nil, err
		}
		return appendAKAChunk(out, "IK", r.IK)
	case len(r.AUTS) != 0:
		if len(r.AUTS) != 14 {
			return nil, fmt.Errorf("marshaling AKA response: invalid AUTS length %d", len(r.AUTS))
		}
		out := []byte{0xDC}
		return appendAKAChunk(out, "AUTS", r.AUTS)
	case r.Reject:
		return []byte{0xDC, 0x00}, nil
	default:
		return nil, errors.New("marshaling AKA response: empty result")
	}
}

func (r *Authenticate3GResult) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return errors.New("parsing AKA response: empty payload")
	}

	original := slices.Clone(data)
	payload := data[1:]
	switch original[0] {
	case 0xDB:
		parsed := Authenticate3GResult{}
		var err error
		parsed.RES, payload, err = readAKAChunk(payload, "RES", original)
		if err != nil {
			return err
		}
		parsed.CK, payload, err = readAKAChunk(payload, "CK", original)
		if err != nil {
			return err
		}
		parsed.IK, payload, err = readAKAChunk(payload, "IK", original)
		if err != nil {
			return err
		}
		*r = parsed
		return nil
	case 0xDC:
		if len(payload) == 0 {
			*r = Authenticate3GResult{Reject: true}
			return nil
		}
		auts, _, err := readAKAChunk(payload, "AUTS", original)
		if err != nil {
			return err
		}
		if len(auts) == 0 {
			*r = Authenticate3GResult{Reject: true}
			return nil
		}
		if len(auts) != 14 {
			return fmt.Errorf("parsing AKA response: invalid AUTS length %d in %X", len(auts), original)
		}
		*r = Authenticate3GResult{AUTS: auts}
		return nil
	default:
		return fmt.Errorf("parsing AKA response: unexpected payload %X", original)
	}
}

func readAKAChunk(data []byte, name string, original []byte) ([]byte, []byte, error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("parsing AKA response: missing %s length in %X", name, original)
	}
	length := int(data[0])
	data = data[1:]
	if len(data) < length {
		return nil, nil, fmt.Errorf("parsing AKA response: truncated %s in %X", name, original)
	}
	chunk := slices.Clone(data[:length])
	return chunk, data[length:], nil
}

func appendAKAChunk(out []byte, name string, value []byte) ([]byte, error) {
	if len(value) > 0xFF {
		return nil, fmt.Errorf("marshaling AKA response: %s exceeds 255 bytes", name)
	}
	out = append(out, byte(len(value)))
	out = append(out, value...)
	return out, nil
}
