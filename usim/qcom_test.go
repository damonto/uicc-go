package usim_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
	"github.com/damonto/uicc-go/qcom/uim"
	"github.com/damonto/uicc-go/usim"
)

type simTransport struct {
	files map[string]simFile
	auth  []byte
}

type simFile struct {
	attrs uim.RawFileAttributes
	data  []byte
	rows  map[uint16][]byte
}

func (t *simTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	switch req.MessageID {
	case qcom.MessageGetFileAttributes:
		session, aid := decodeSessionTLV(req.TLVs)
		path := decodeFileTLV(req.TLVs)
		file, ok := t.files[simFileKey(session, aid, path)]
		if !ok {
			return errorResponse(qcom.MessageGetFileAttributes, qcom.QMIErrorSimFileNotFound), nil
		}
		return successResponse(qcom.MessageGetFileAttributes,
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, encodeFileAttributes(file.attrs.FileSize, file.attrs.FileID, byte(file.attrs.FileType), file.attrs.RecordSize, file.attrs.RecordCount, file.attrs.Raw)),
		), nil
	case qcom.MessageReadTransparent:
		session, aid := decodeSessionTLV(req.TLVs)
		path := decodeFileTLV(req.TLVs)
		file, ok := t.files[simFileKey(session, aid, path)]
		if !ok || len(file.data) == 0 {
			return errorResponse(qcom.MessageReadTransparent, qcom.QMIErrorSimFileNotFound), nil
		}
		return successResponse(qcom.MessageReadTransparent,
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, encodeLengthPrefixed(file.data)),
		), nil
	case qcom.MessageReadRecord:
		session, aid := decodeSessionTLV(req.TLVs)
		path := decodeFileTLV(req.TLVs)
		record := decodeRecordTLV(req.TLVs)
		file, ok := t.files[simFileKey(session, aid, path)]
		if !ok {
			return errorResponse(qcom.MessageReadRecord, qcom.QMIErrorSimFileNotFound), nil
		}
		row, ok := file.rows[record]
		if !ok {
			return errorResponse(qcom.MessageReadRecord, qcom.QMIErrorSimFileNotFound), nil
		}
		return successResponse(qcom.MessageReadRecord,
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, encodeLengthPrefixed(row)),
		), nil
	case qcom.MessageAuthenticate:
		return successResponse(qcom.MessageAuthenticate,
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, encodeLengthPrefixed(t.auth)),
		), nil
	default:
		return qcom.Response{}, fmt.Errorf("unexpected message 0x%04X", req.MessageID)
	}
}

func (t *simTransport) Close() error {
	return nil
}

func TestNewWithFakeQMITransport(t *testing.T) {
	tests := []struct {
		name      string
		transport *simTransport
		check     func(t *testing.T, card *usim.Card)
	}{
		{
			name:      "usim only with missing optional files",
			transport: newUSIMOnlyTransport(),
			check: func(t *testing.T, card *usim.Card) {
				t.Helper()
				if got := card.ICCID(); got != "8986000000000000000" {
					t.Fatalf("ICCID() = %q", got)
				}
				if got := card.IMSI(); got != "001010123456789" {
					t.Fatalf("IMSI() = %q", got)
				}
				if got := card.MNC(); got != "001" {
					t.Fatalf("MNC() = %q", got)
				}
				if got := card.SMSC(); got != "" {
					t.Fatalf("SMSC() = %q, want empty", got)
				}
				if got := card.PrivateIdentity(); got != "" {
					t.Fatalf("PrivateIdentity() = %q, want empty", got)
				}
			},
		},
		{
			name:      "usim and isim",
			transport: newUSIMISIMTransport(),
			check: func(t *testing.T, card *usim.Card) {
				t.Helper()
				if got := card.PrivateIdentity(); got != "alice@ims.example.com" {
					t.Fatalf("PrivateIdentity() = %q", got)
				}
				if got := card.PublicIdentity(); got != "sip:alice@ims.example.com" {
					t.Fatalf("PublicIdentity() = %q", got)
				}
				if got := card.HomeDomain(); got != "ims.example.com" {
					t.Fatalf("HomeDomain() = %q", got)
				}
				if got := card.ServiceCenter().PSI; got != "sip:isim-smsc@example.com" {
					t.Fatalf("ServiceCenter().PSI = %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := uim.New(context.Background(), tt.transport, uim.WithSlot(1), uim.WithClientID(1))
			if err != nil {
				t.Fatalf("uim.New() error = %v", err)
			}
			adapter, err := usim.NewQCOM(reader)
			if err != nil {
				t.Fatalf("NewQCOM() error = %v", err)
			}
			card, err := usim.New(context.Background(), adapter)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			tt.check(t, card)
		})
	}
}

func TestAKAWithFakeQMITransport(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		check func(t *testing.T, got usim.AKAResult)
	}{
		{
			name: "success",
			body: []byte{
				0xDB, 0x04, 0x11, 0x22, 0x33, 0x44,
				0x10, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA,
				0x10, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB,
			},
			check: func(t *testing.T, got usim.AKAResult) {
				t.Helper()
				if !got.IsSuccess() {
					t.Fatalf("AKA() = %+v, want success", got)
				}
			},
		},
		{
			name: "sync failure",
			body: append([]byte{0xDC, 0x0E}, bytes.Repeat([]byte{0xAA}, 14)...),
			check: func(t *testing.T, got usim.AKAResult) {
				t.Helper()
				if !got.IsSynchronizationFailure() {
					t.Fatalf("AKA() = %+v, want sync failure", got)
				}
			},
		},
		{
			name: "reject",
			body: []byte{0xDC, 0x00},
			check: func(t *testing.T, got usim.AKAResult) {
				t.Helper()
				if !got.IsAuthenticationReject() {
					t.Fatalf("AKA() = %+v, want reject", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := uim.New(context.Background(), &simTransport{
				files: newUSIMOnlyTransport().files,
				auth:  tt.body,
			}, uim.WithSlot(1), uim.WithClientID(1))
			if err != nil {
				t.Fatalf("uim.New() error = %v", err)
			}
			adapter, err := usim.NewQCOM(reader)
			if err != nil {
				t.Fatalf("NewQCOM() error = %v", err)
			}
			card, err := usim.New(context.Background(), adapter)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := card.AKA(context.Background(), bytes.Repeat([]byte{0x01}, 16), bytes.Repeat([]byte{0x02}, 16))
			if err != nil {
				t.Fatalf("AKA() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func newUSIMOnlyTransport() *simTransport {
	files := map[string]simFile{
		simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x2F, 0x00}): {
			attrs: makeAttrs(20, 0x2F00, 2, 20, 1, "620B8205422100140180020014"),
			rows: map[uint16][]byte{
				1: mustHex("610F4F07A000000087100250045553494D"),
			},
		},
		simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x2F, 0xE2}): {
			attrs: makeAttrs(10, 0x2FE2, 0, 0, 0, "6208820241218002000A"),
			data:  mustHex("986800000000000000F0"),
		},
		simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x07}): {
			attrs: makeAttrs(9, 0x6F07, 0, 0, 0, "62088202412180020009"),
			data:  mustHex("080910101032547698"),
		},
		simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD}): {
			attrs: makeAttrs(4, 0x6FAD, 0, 0, 0, "62088202412180020004"),
			data:  []byte{0x00, 0x00, 0x00, 0x02},
		},
	}
	return &simTransport{files: files, auth: []byte{0xDC, 0x00}}
}

func newUSIMISIMTransport() *simTransport {
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	files := newUSIMOnlyTransport().files
	files[simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x2F, 0x00})] = simFile{
		attrs: makeAttrs(40, 0x2F00, 2, 20, 2, "620B8205422100140280020028"),
		rows: map[uint16][]byte{
			1: mustHex("610F4F07A000000087100250045553494D"),
			2: mustHex("610F4F07A000000087100450044953494D"),
		},
	}
	files[simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x42})] = simFile{
		attrs: makeAttrs(28, 0x6F42, 2, 28, 1, "620B82054221001C018002001C"),
		rows: map[uint16][]byte{
			1: smscRecord(28, 0x91, 0x55, 0x15, 0x00, 0x00, 0x00, 0xF0),
		},
	}
	files[simFileKey(uim.SessionPrimaryGWProvisioning, nil, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x7F, 0x10, 0x6F, 0xE5})] = simFile{
		attrs: makeAttrs(32, 0x6FE5, 2, 32, 1, "620B8205422100200180020020"),
		rows: map[uint16][]byte{
			1: tlvTextRecord("sip:usim-smsc@example.com", 32),
		},
	}
	files[simFileKey(uim.SessionCardSlot1, isimAID, []byte{0x6F, 0x02})] = simFile{
		attrs: makeAttrs(32, 0x6F02, 0, 0, 0, "62088202412180020020"),
		data:  tlvTextBinary("alice@ims.example.com", 32),
	}
	files[simFileKey(uim.SessionCardSlot1, isimAID, []byte{0x6F, 0x04})] = simFile{
		attrs: makeAttrs(32, 0x6F04, 2, 32, 1, "620B8205422100200180020020"),
		rows: map[uint16][]byte{
			1: tlvTextRecord("sip:alice@ims.example.com", 32),
		},
	}
	files[simFileKey(uim.SessionCardSlot1, isimAID, []byte{0x7F, 0x10, 0x6F, 0xE5})] = simFile{
		attrs: makeAttrs(32, 0x6FE5, 2, 32, 1, "620B8205422100200180020020"),
		rows: map[uint16][]byte{
			1: tlvTextRecord("sip:isim-smsc@example.com", 32),
		},
	}
	return &simTransport{files: files, auth: []byte{0xDC, 0x00}}
}

func decodeSessionTLV(tlvs tlv.TLVs) (uim.Session, []byte) {
	value, _ := tlv.Value(tlvs, 0x01)
	if len(value) < 2 {
		return 0, nil
	}
	return uim.Session(value[0]), bytes.Clone(value[2:])
}

func decodeFileTLV(tlvs tlv.TLVs) []byte {
	value, _ := tlv.Value(tlvs, 0x02)
	if len(value) < 3 {
		return nil
	}

	fileID := binary.LittleEndian.Uint16(value[:2])
	pathLength := int(value[2])
	path := make([]byte, 0, pathLength+2)
	for i := 0; i < pathLength; i += 2 {
		path = append(path, value[3+i+1], value[3+i])
	}
	return binary.BigEndian.AppendUint16(path, fileID)
}

func decodeRecordTLV(tlvs tlv.TLVs) uint16 {
	value, _ := tlv.Value(tlvs, 0x03)
	if len(value) < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(value[:2])
}

func simFileKey(session uim.Session, aid, path []byte) string {
	return fmt.Sprintf("%d:%s:%s", session, hex.EncodeToString(aid), hex.EncodeToString(path))
}

func makeAttrs(fileSize, fileID uint16, fileType uim.QMIFileType, recordSize, recordCount uint16, rawHex string) uim.RawFileAttributes {
	return uim.RawFileAttributes{
		FileSize:    fileSize,
		FileID:      fileID,
		FileType:    fileType,
		RecordSize:  recordSize,
		RecordCount: recordCount,
		Raw:         mustHex(rawHex),
	}
}

func mustHex(s string) []byte {
	data, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return data
}

func tlvTextBinary(value string, size int) []byte {
	data := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(data) < size {
		data = append(data, 0xFF)
	}
	return data
}

func smscRecord(size int, toa byte, digits ...byte) []byte {
	record := make([]byte, size)
	start := size - 28 + 13
	record[start] = byte(len(digits) + 1)
	record[start+1] = toa
	copy(record[start+2:], digits)
	return record
}

func successResponse(id qcom.MessageID, tlvs ...tlv.TLV) qcom.Response {
	return qcom.Response{
		Service:   qcom.ServiceUIM,
		ClientID:  1,
		MessageID: id,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(0x02, []byte{0x00, 0x00, 0x00, 0x00}),
		}, tlvs...),
	}
}

func errorResponse(id qcom.MessageID, err qcom.QMIError) qcom.Response {
	return qcom.Response{
		Service:   qcom.ServiceUIM,
		ClientID:  1,
		MessageID: id,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x02, []byte{0x01, 0x00, byte(err), byte(uint16(err) >> 8)}),
		},
	}
}

func encodeLengthPrefixed(data []byte) []byte {
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(data))), data...)
}

func encodeFileAttributes(fileSize, fileID uint16, fileType byte, recordSize, recordCount uint16, raw []byte) []byte {
	value := binary.LittleEndian.AppendUint16(nil, fileSize)
	value = binary.LittleEndian.AppendUint16(value, fileID)
	value = append(value, fileType)
	value = binary.LittleEndian.AppendUint16(value, recordSize)
	value = binary.LittleEndian.AppendUint16(value, recordCount)
	for range 5 {
		value = append(value, 0x00)
		value = binary.LittleEndian.AppendUint16(value, 0x0000)
	}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(raw)))
	value = append(value, raw...)
	return value
}

func tlvTextRecord(value string, size int) []byte {
	record := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(record) < size {
		record = append(record, 0xFF)
	}
	return record
}
