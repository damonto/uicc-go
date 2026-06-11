package tlv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
)

type TLV struct {
	Type  uint8
	Len   uint16
	Value []byte
}

type TLVs []TLV

func (ts *TLVs) ReadFrom(r io.Reader) (int64, error) {
	items := make(TLVs, 0)
	var read int64
	for {
		var t uint8
		if err := binary.Read(r, binary.LittleEndian, &t); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return read, fmt.Errorf("read TLV type: %w", err)
		}
		read++

		var n uint16
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return read, fmt.Errorf("read TLV length: %w", err)
		}
		read += 2

		v := make([]byte, n)
		nr, err := io.ReadFull(r, v)
		read += int64(nr)
		if err != nil {
			return read, fmt.Errorf("read TLV value: %w", err)
		}
		items = append(items, TLV{Type: t, Len: n, Value: v})
	}
	*ts = items
	return read, nil
}

func (ts TLVs) WriteTo(w io.Writer) (int64, error) {
	var written int64
	for _, tlv := range ts {
		if int(tlv.Len) != len(tlv.Value) {
			return written, fmt.Errorf("TLV type 0x%02X length mismatch: header %d, value %d", tlv.Type, tlv.Len, len(tlv.Value))
		}
		if err := binary.Write(w, binary.LittleEndian, tlv.Type); err != nil {
			return written, fmt.Errorf("write TLV type: %w", err)
		}
		written++
		if err := binary.Write(w, binary.LittleEndian, tlv.Len); err != nil {
			return written, fmt.Errorf("write TLV length: %w", err)
		}
		written += 2
		n, err := w.Write(tlv.Value)
		written += int64(n)
		if err != nil {
			return written, fmt.Errorf("write TLV value: %w", err)
		}
		if n != len(tlv.Value) {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (ts TLVs) Find(t uint8) (TLV, bool) {
	for _, tlv := range ts {
		if tlv.Type == t {
			return tlv, true
		}
	}
	return TLV{}, false
}

type uintValue interface {
	~uint8 | ~uint16 | ~uint32
}

func Bytes(typ byte, value []byte) TLV {
	return TLV{Type: typ, Len: uint16(len(value)), Value: slices.Clone(value)}
}

func Uint[T uintValue](typ byte, value T) TLV {
	data, err := binary.Append(nil, binary.LittleEndian, value)
	if err != nil {
		panic(fmt.Sprintf("encoding unsigned TLV value: %v", err))
	}
	return TLV{Type: typ, Len: uint16(len(data)), Value: data}
}

func Value(tlvs TLVs, typ byte) ([]byte, bool) {
	item, ok := tlvs.Find(typ)
	if !ok {
		return nil, false
	}
	return slices.Clone(item.Value), true
}
