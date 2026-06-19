package command

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"strings"
	"testing"

	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/simfile"
)

type fakeReader struct {
	listApplications  func(context.Context) ([]usimcard.Application, error)
	getFileAttributes func(context.Context, usimcard.FileRef) (usimcard.FileAttributes, error)
	readTransparent   func(context.Context, usimcard.TransparentRead) ([]byte, error)
	readRecord        func(context.Context, usimcard.RecordRead) ([]byte, error)
	authenticate3G    func(context.Context, usimcard.AuthenticateRequest) ([]byte, error)
	smsPPDownload     func(context.Context, usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error)
}

func (r *fakeReader) ListApplications(ctx context.Context) ([]usimcard.Application, error) {
	if r.listApplications == nil {
		return nil, errors.New("ListApplications was not expected")
	}
	return r.listApplications(ctx)
}

func (r *fakeReader) FileAttributes(ctx context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
	if r.getFileAttributes == nil {
		return usimcard.FileAttributes{}, errors.New("FileAttributes was not expected")
	}
	return r.getFileAttributes(ctx, file)
}

func (r *fakeReader) ReadTransparent(ctx context.Context, req usimcard.TransparentRead) ([]byte, error) {
	if r.readTransparent == nil {
		return nil, errors.New("ReadTransparent was not expected")
	}
	return r.readTransparent(ctx, req)
}

func (r *fakeReader) ReadRecord(ctx context.Context, req usimcard.RecordRead) ([]byte, error) {
	if r.readRecord == nil {
		return nil, errors.New("ReadRecord was not expected")
	}
	return r.readRecord(ctx, req)
}

func (r *fakeReader) Authenticate3G(ctx context.Context, req usimcard.AuthenticateRequest) ([]byte, error) {
	if r.authenticate3G == nil {
		return nil, errors.New("Authenticate3G was not expected")
	}
	return r.authenticate3G(ctx, req)
}

func (r *fakeReader) SMSPPDownload(ctx context.Context, req usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error) {
	if r.smsPPDownload == nil {
		return usimcard.SMSPPDownloadResponse{}, errors.New("SMSPPDownload was not expected")
	}
	return r.smsPPDownload(ctx, req)
}

func (r *fakeReader) Close() error { return nil }

func TestReadCommandsMarshalBinary(t *testing.T) {
	tests := []struct {
		name string
		cmd  encoding.BinaryMarshaler
		want []byte
	}{
		{
			name: "binary read",
			cmd:  BinaryRead{Offset: 0x1234, Length: 0x10},
			want: []byte{0x00, 0xB0, 0x12, 0x34, 0x10},
		},
		{
			name: "record read",
			cmd:  RecordRead{Record: 0x03, Length: 0x20},
			want: []byte{0x00, 0xB2, 0x03, 0x04, 0x20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cmd.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestAuthenticate3GResultBinary(t *testing.T) {
	res := []byte{0x11, 0x22, 0x33, 0x44}
	ck := bytes.Repeat([]byte{0xAA}, 16)
	ik := bytes.Repeat([]byte{0xBB}, 16)
	auts := bytes.Repeat([]byte{0xCC}, 14)
	akaSuccess := append(
		append(
			append(
				append([]byte{0xDB, byte(len(res))}, res...),
				byte(len(ck)),
			),
			ck...,
		),
		append([]byte{byte(len(ik))}, ik...)...,
	)
	akaSyncFailure := append([]byte{0xDC, byte(len(auts))}, auts...)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "success",
			check: func(t *testing.T) {
				var got Authenticate3GResult
				if err := got.UnmarshalBinary(akaSuccess); err != nil {
					t.Fatalf("UnmarshalBinary() error = %v", err)
				}
				if !bytes.Equal(got.RES, res) || !bytes.Equal(got.CK, ck) || !bytes.Equal(got.IK, ik) {
					t.Fatalf("UnmarshalBinary() = %+v", got)
				}
				encoded, err := got.MarshalBinary()
				if err != nil {
					t.Fatalf("MarshalBinary() error = %v", err)
				}
				if !bytes.Equal(encoded, akaSuccess) {
					t.Fatalf("MarshalBinary() = % X, want % X", encoded, akaSuccess)
				}
			},
		},
		{
			name: "sync failure",
			check: func(t *testing.T) {
				var got Authenticate3GResult
				if err := got.UnmarshalBinary(akaSyncFailure); err != nil {
					t.Fatalf("UnmarshalBinary() error = %v", err)
				}
				if !got.IsSynchronizationFailure() || !bytes.Equal(got.AUTS, auts) {
					t.Fatalf("UnmarshalBinary() = %+v", got)
				}
				encoded, err := got.MarshalBinary()
				if err != nil {
					t.Fatalf("MarshalBinary() error = %v", err)
				}
				if !bytes.Equal(encoded, akaSyncFailure) {
					t.Fatalf("MarshalBinary() = % X, want % X", encoded, akaSyncFailure)
				}
			},
		},
		{
			name: "reject",
			check: func(t *testing.T) {
				var got Authenticate3GResult
				if err := got.UnmarshalBinary([]byte{0xDC, 0x00}); err != nil {
					t.Fatalf("UnmarshalBinary() error = %v", err)
				}
				if !got.IsAuthenticationReject() {
					t.Fatalf("UnmarshalBinary() = %+v, want reject", got)
				}
				encoded, err := got.MarshalBinary()
				if err != nil {
					t.Fatalf("MarshalBinary() error = %v", err)
				}
				if !bytes.Equal(encoded, []byte{0xDC, 0x00}) {
					t.Fatalf("MarshalBinary() = % X, want DC 00", encoded)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func TestFindAIDRun(t *testing.T) {
	reader := &fakeReader{
		listApplications: func(context.Context) ([]usimcard.Application, error) {
			return []usimcard.Application{
				{AID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}, Label: "USIM"},
			}, nil
		},
	}

	got, err := FindAID{
		Label:    "USIM",
		Prefix:   []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
		NotFound: errors.New("not found"),
	}.Run(context.Background(), reader)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	if !bytes.Equal(got, want) {
		t.Fatalf("Run() = %X, want %X", got, want)
	}
}

func TestAppReaders(t *testing.T) {
	usimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "transparent file",
			check: func(t *testing.T) {
				data := []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0}
				app := App{Reader: &fakeReader{
					getFileAttributes: func(_ context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
						if !bytes.Equal(file.AID, usimAID) || !bytes.Equal(file.Path, []byte{0x2F, 0xE2}) {
							t.Fatalf("FileAttributes() file = %+v", file)
						}
						return usimcard.FileAttributes{FileStructure: simfile.StructureTransparent, FileSize: 10}, nil
					},
					readTransparent: func(_ context.Context, req usimcard.TransparentRead) ([]byte, error) {
						if req.Length != 10 {
							t.Fatalf("ReadTransparent() length = %d, want 10", req.Length)
						}
						return data, nil
					},
				}, AID: usimAID}

				want := []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0}
				got, err := app.Transparent(context.Background(), []byte{0x2F, 0xE2}, "reading EF_ICCID")
				if err != nil {
					t.Fatalf("Transparent() error = %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("Transparent() = % X, want % X", got, want)
				}
				clear(data)
				if !bytes.Equal(got, want) {
					t.Fatalf("Transparent() result aliases reader buffer: % X", got)
				}
			},
		},
		{
			name: "read first linear fixed text record",
			check: func(t *testing.T) {
				app := App{Reader: &fakeReader{
					getFileAttributes: func(_ context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
						if !bytes.Equal(file.AID, isimAID) || !bytes.Equal(file.Path, []byte{0x6F, 0x04}) {
							t.Fatalf("FileAttributes() file = %+v", file)
						}
						return usimcard.FileAttributes{FileStructure: simfile.StructureLinearFixed, RecordSize: 16, RecordCount: 2}, nil
					},
					readRecord: func(_ context.Context, req usimcard.RecordRead) ([]byte, error) {
						if req.Record != 1 || req.Length != 16 {
							t.Fatalf("ReadRecord() req = %+v", req)
						}
						return append([]byte{0x80, 0x0C}, []byte("sip:test@ims\xFF\xFF")...), nil
					},
				}, AID: isimAID}

				got, err := app.FirstText(context.Background(), []byte{0x6F, 0x04}, "reading EF_IMPU")
				if err != nil {
					t.Fatalf("FirstText() error = %v", err)
				}
				if got != "sip:test@ims" {
					t.Fatalf("FirstText() = %q, want %q", got, "sip:test@ims")
				}
			},
		},
		{
			name: "skip empty first record",
			check: func(t *testing.T) {
				app := App{Reader: &fakeReader{
					getFileAttributes: func(context.Context, usimcard.FileRef) (usimcard.FileAttributes, error) {
						return usimcard.FileAttributes{FileStructure: simfile.StructureLinearFixed, RecordSize: 16, RecordCount: 2}, nil
					},
					readRecord: func(_ context.Context, req usimcard.RecordRead) ([]byte, error) {
						switch req.Record {
						case 1:
							return []byte{0xFF, 0xFF, 0xFF}, nil
						case 2:
							return append([]byte{0x80, 0x0C}, []byte("sip:next@ims\xFF\xFF")...), nil
						default:
							t.Fatalf("ReadRecord() unexpected record %d", req.Record)
							return nil, nil
						}
					},
				}, AID: isimAID}

				got, err := app.FirstText(context.Background(), []byte{0x6F, 0x04}, "reading EF_IMPU")
				if err != nil {
					t.Fatalf("FirstText() error = %v", err)
				}
				if got != "sip:next@ims" {
					t.Fatalf("FirstText() = %q, want %q", got, "sip:next@ims")
				}
			},
		},
		{
			name: "linear fixed clones records",
			check: func(t *testing.T) {
				buf := make([]byte, 4)
				app := App{Reader: &fakeReader{
					getFileAttributes: func(context.Context, usimcard.FileRef) (usimcard.FileAttributes, error) {
						return usimcard.FileAttributes{FileStructure: simfile.StructureLinearFixed, RecordSize: uint16(len(buf)), RecordCount: 2}, nil
					},
					readRecord: func(_ context.Context, req usimcard.RecordRead) ([]byte, error) {
						for i := range buf {
							buf[i] = byte(req.Record)
						}
						return buf, nil
					},
				}, AID: isimAID}

				got, err := app.LinearFixed(context.Background(), []byte{0x6F, 0x04}, "reading EF_IMPU")
				if err != nil {
					t.Fatalf("LinearFixed() error = %v", err)
				}
				if len(got) != 2 {
					t.Fatalf("len(LinearFixed()) = %d, want 2", len(got))
				}
				if !bytes.Equal(got[0], []byte{1, 1, 1, 1}) || !bytes.Equal(got[1], []byte{2, 2, 2, 2}) {
					t.Fatalf("LinearFixed() = % X", got)
				}
			},
		},
		{
			name: "no populated record is an error",
			check: func(t *testing.T) {
				app := App{Reader: &fakeReader{
					getFileAttributes: func(context.Context, usimcard.FileRef) (usimcard.FileAttributes, error) {
						return usimcard.FileAttributes{FileStructure: simfile.StructureLinearFixed, RecordSize: 16, RecordCount: 2}, nil
					},
					readRecord: func(context.Context, usimcard.RecordRead) ([]byte, error) {
						return []byte{0xFF, 0xFF, 0xFF}, nil
					},
				}, AID: isimAID}

				_, err := app.FirstText(context.Background(), []byte{0x6F, 0x04}, "reading EF_IMPU")
				if err == nil {
					t.Fatal("FirstText() error = nil, want non-nil")
				}
				if !strings.Contains(err.Error(), "no populated record") {
					t.Fatalf("FirstText() error = %v, want no populated record", err)
				}
			},
		},
		{
			name: "read telecom psi smsc by path",
			check: func(t *testing.T) {
				app := App{Reader: &fakeReader{
					getFileAttributes: func(_ context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
						if !bytes.Equal(file.Path, []byte{0x7F, 0x10, 0x6F, 0xE5}) {
							t.Fatalf("FileAttributes() file = %+v", file)
						}
						return usimcard.FileAttributes{FileStructure: simfile.StructureLinearFixed, RecordSize: 32, RecordCount: 1}, nil
					},
					readRecord: func(_ context.Context, req usimcard.RecordRead) ([]byte, error) {
						if req.Record != 1 || req.Length != 32 {
							t.Fatalf("ReadRecord() req = %+v", req)
						}
						return tlvTextRecord("sip:smsc@example.com", 32), nil
					},
				}, AID: usimAID}

				got, err := app.FirstText(context.Background(), []byte{0x7F, 0x10, 0x6F, 0xE5}, "reading EFPSISMSC")
				if err != nil {
					t.Fatalf("FirstText() error = %v", err)
				}
				if got != "sip:smsc@example.com" {
					t.Fatalf("FirstText() = %q, want %q", got, "sip:smsc@example.com")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func tlvTextRecord(value string, size int) []byte {
	record := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(record) < size {
		record = append(record, 0xFF)
	}
	return record
}
