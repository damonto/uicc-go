package tlv

import (
	"bytes"
	"testing"
)

type testUint8 uint8

func TestUint(t *testing.T) {
	tests := []struct {
		name string
		tlv  TLV
		want TLV
	}{
		{
			name: "uint8",
			tlv:  Uint(0x01, uint8(0x7F)),
			want: TLV{Type: 0x01, Len: 1, Value: []byte{0x7F}},
		},
		{
			name: "defined uint8",
			tlv:  Uint(0x02, testUint8(0x80)),
			want: TLV{Type: 0x02, Len: 1, Value: []byte{0x80}},
		},
		{
			name: "uint16 little endian",
			tlv:  Uint(0x03, uint16(0x1234)),
			want: TLV{Type: 0x03, Len: 2, Value: []byte{0x34, 0x12}},
		},
		{
			name: "uint32 little endian",
			tlv:  Uint(0x04, uint32(0x01020304)),
			want: TLV{Type: 0x04, Len: 4, Value: []byte{0x04, 0x03, 0x02, 0x01}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tlv.Type != tt.want.Type || tt.tlv.Len != tt.want.Len || !bytes.Equal(tt.tlv.Value, tt.want.Value) {
				t.Fatalf("Uint() = %+v, want %+v", tt.tlv, tt.want)
			}
		})
	}
}

func TestBytesClonesValue(t *testing.T) {
	value := []byte{0x01, 0x02}
	got := Bytes(0x10, value)
	value[0] = 0xff

	if !bytes.Equal(got.Value, []byte{0x01, 0x02}) {
		t.Fatalf("Bytes() value = % X, want 01 02", got.Value)
	}
}

func TestValueClonesValue(t *testing.T) {
	tlvs := TLVs{Bytes(0x10, []byte{0x01, 0x02})}
	got, ok := Value(tlvs, 0x10)
	if !ok {
		t.Fatal("Value() ok = false, want true")
	}
	got[0] = 0xff

	again, ok := Value(tlvs, 0x10)
	if !ok {
		t.Fatal("Value() ok = false, want true")
	}
	if !bytes.Equal(again, []byte{0x01, 0x02}) {
		t.Fatalf("Value() = % X, want 01 02", again)
	}
}
