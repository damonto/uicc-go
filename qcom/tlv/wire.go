package tlv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
)

func (ts *TLVs) UnmarshalBinary(data []byte) error {
	items := make(TLVs, 0, len(data)/4)
	for len(data) > 0 {
		if len(data) < 3 {
			return errors.New("truncated TLV header")
		}

		length := int(binary.LittleEndian.Uint16(data[1:3]))
		if len(data) < 3+length {
			return errors.New("truncated TLV value")
		}

		items = append(items, TLV{
			Type:  data[0],
			Len:   uint16(length),
			Value: slices.Clone(data[3 : 3+length]),
		})
		data = data[3+length:]
	}
	*ts = items
	return nil
}

func (ts TLVs) MarshalBinary() ([]byte, error) {
	size := 0
	for _, tlv := range ts {
		size += 3 + len(tlv.Value)
	}

	var buf bytes.Buffer
	buf.Grow(size)
	if _, err := ts.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
