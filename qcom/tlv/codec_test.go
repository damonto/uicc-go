package tlv

import (
	"bytes"
	"encoding"
	"io"
	"testing"
)

func TestTLVsImplementBinaryMarshaler(t *testing.T) {
	var _ encoding.BinaryMarshaler = TLVs{}
	var _ encoding.BinaryUnmarshaler = (*TLVs)(nil)
	var _ io.WriterTo = TLVs{}
	var _ io.ReaderFrom = (*TLVs)(nil)
}

func TestTLVMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    TLVs
		want    []byte
		wantErr bool
	}{
		{
			name: "valid",
			tlvs: TLVs{
				{Type: 0x02, Len: 4, Value: []byte{0x00, 0x00, 0x00, 0x00}},
				{Type: 0x10, Len: 1, Value: []byte{0xaa}},
			},
			want: []byte{
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x10, 0x01, 0x00, 0xaa,
			},
		},
		{
			name:    "length mismatch",
			tlvs:    TLVs{{Type: 0x10, Len: 2, Value: []byte{0xaa}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tlvs.MarshalBinary()
			if tt.wantErr {
				if err == nil {
					t.Fatal("MarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestTLVReadFromReturnsWireLength(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		initial TLVs
		want    TLVs
	}{
		{
			name: "response TLVs",
			data: []byte{
				0x02, 0x04, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x10, 0x01, 0x00,
				0xaa,
			},
			want: TLVs{
				{Type: 0x02, Len: 4, Value: []byte{0x00, 0x00, 0x00, 0x00}},
				{Type: 0x10, Len: 1, Value: []byte{0xaa}},
			},
		},
		{
			name:    "plain TLVs",
			initial: TLVs{{Type: 0x99, Len: 1, Value: []byte{0xbb}}},
			data: []byte{
				0x10, 0x01, 0x00,
				0xaa,
			},
			want: TLVs{{Type: 0x10, Len: 1, Value: []byte{0xaa}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlvs := tt.initial
			n, err := tlvs.ReadFrom(bytes.NewReader(tt.data))
			if err != nil {
				t.Fatalf("ReadFrom failed: %v", err)
			}
			if n != int64(len(tt.data)) {
				t.Fatalf("ReadFrom length = %d, want %d", n, len(tt.data))
			}
			if len(tlvs) != len(tt.want) {
				t.Fatalf("ReadFrom items = %+v, want %+v", tlvs, tt.want)
			}
			for i := range tt.want {
				if tlvs[i].Type != tt.want[i].Type || tlvs[i].Len != tt.want[i].Len || !bytes.Equal(tlvs[i].Value, tt.want[i].Value) {
					t.Fatalf("ReadFrom item %d = %+v, want %+v", i, tlvs[i], tt.want[i])
				}
			}
		})
	}
}

func TestTLVWriteToReturnsWireLength(t *testing.T) {
	tlvs := TLVs{
		{Type: 0x02, Len: 4, Value: []byte{0x00, 0x00, 0x00, 0x00}},
		{Type: 0x10, Len: 1, Value: []byte{0xaa}},
	}
	var buf bytes.Buffer

	n, err := tlvs.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n != int64(buf.Len()) {
		t.Fatalf("WriteTo length = %d, want %d", n, buf.Len())
	}
}

func TestTLVWriteToRejectsLengthMismatch(t *testing.T) {
	tlvs := TLVs{{Type: 0x10, Len: 2, Value: []byte{0xaa}}}

	_, err := tlvs.WriteTo(io.Discard)
	if err == nil {
		t.Fatal("WriteTo error = nil, want length mismatch")
	}
}
