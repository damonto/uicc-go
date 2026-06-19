package usim

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/apdu"
	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/simfile"
)

var (
	masterFile = []byte{0x3F, 0x00}
	efDirFile  = []byte{0x2F, 0x00}
)

type Reader struct {
	tx usimcard.Transmitter

	selected selection
}

type selection struct {
	valid bool
	file  usimcard.FileRef
	info  simfile.FCI
}

func NewReader(tx usimcard.Transmitter) (*Reader, error) {
	if tx == nil {
		return nil, errors.New("creating APDU USIM reader: transmitter is nil")
	}
	return &Reader{tx: tx}, nil
}

func (r *Reader) ListApplications(ctx context.Context) ([]usimcard.Application, error) {
	if _, err := r.selectID(ctx, masterFile); err != nil {
		return nil, err
	}

	info, err := r.selectID(ctx, efDirFile)
	if err != nil {
		return nil, fmt.Errorf("reading EF_DIR: %w", err)
	}
	if info.FileStructure != simfile.StructureLinearFixed {
		return nil, errors.New("reading EF_DIR: unexpected file structure")
	}

	apps := make([]usimcard.Application, 0, info.RecordCount)
	for recordID := uint16(1); recordID <= uint16(info.RecordCount); recordID++ {
		record, err := r.readRecordRaw(ctx, recordID, info.RecordSize)
		if err != nil {
			return nil, fmt.Errorf("parsing EF_DIR record %d: %w", recordID, err)
		}

		var parsed simfile.EFDirRecord
		if err := parsed.UnmarshalBinary(record); err != nil {
			return nil, fmt.Errorf("parsing EF_DIR record %d: %w", recordID, err)
		}
		if len(parsed.AID) == 0 {
			continue
		}
		apps = append(apps, usimcard.Application{
			AID:   slices.Clone(parsed.AID),
			Label: parsed.Label,
		})
	}
	return apps, nil
}

func (r *Reader) FileAttributes(ctx context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
	info, err := r.selectFile(ctx, file)
	if err != nil {
		return usimcard.FileAttributes{}, err
	}
	return usimcard.FileAttributes{
		FileStructure: info.FileStructure,
		FileType:      info.FileType,
		RecordSize:    info.RecordSize,
		RecordCount:   uint16(info.RecordCount),
		FileSize:      info.FileSize,
	}, nil
}

func (r *Reader) ReadTransparent(ctx context.Context, req usimcard.TransparentRead) ([]byte, error) {
	info, err := r.selectFile(ctx, req.File)
	if err != nil {
		return nil, err
	}
	if info.FileStructure != simfile.StructureTransparent {
		return nil, errors.New("reading transparent file: unexpected file structure")
	}

	length := req.Length
	if length == 0 {
		if req.Offset > info.FileSize {
			return nil, errors.New("reading transparent file: offset exceeds file size")
		}
		length = info.FileSize - req.Offset
	}
	return r.readBinaryRaw(ctx, req.Offset, length)
}

func (r *Reader) ReadRecord(ctx context.Context, req usimcard.RecordRead) ([]byte, error) {
	info, err := r.selectFile(ctx, req.File)
	if err != nil {
		return nil, err
	}
	if info.FileStructure != simfile.StructureLinearFixed {
		return nil, errors.New("reading record file: unexpected file structure")
	}

	length := req.Length
	if length == 0 {
		length = info.RecordSize
	}
	return r.readRecordRaw(ctx, req.Record, length)
}

func (r *Reader) Authenticate3G(ctx context.Context, req usimcard.AuthenticateRequest) ([]byte, error) {
	if len(req.AID) != 0 {
		if _, err := r.selectName(ctx, req.AID); err != nil {
			return nil, fmt.Errorf("selecting authentication application: %w", err)
		}
		r.selected = selection{}
	}
	raw, err := r.transmitCommand(ctx, command.Authenticate3G{
		Rand: slices.Clone(req.Rand),
		AUTN: slices.Clone(req.AUTN),
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(raw.Data()), nil
}

func (r *Reader) SMSPPDownload(ctx context.Context, req usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error) {
	raw, err := r.transmitEnvelopeCommand(ctx, command.SMSPPDownload{
		ServiceCenterAddress: req.ServiceCenterAddress,
		TPDU:                 slices.Clone(req.TPDU),
	})
	if err != nil {
		return usimcard.SMSPPDownloadResponse{}, err
	}
	return usimcard.SMSPPDownloadResponse{
		SW1:  raw.SW1(),
		SW2:  raw.SW2(),
		Data: slices.Clone(raw.Data()),
	}, nil
}

func (r *Reader) Close() error {
	return r.tx.Close()
}

func (r *Reader) selectFile(ctx context.Context, file usimcard.FileRef) (simfile.FCI, error) {
	if len(file.Path) == 0 {
		return simfile.FCI{}, errors.New("selecting file: path is empty")
	}
	if r.selected.matches(file) {
		return r.selected.info, nil
	}

	if len(file.AID) > 0 {
		if _, err := r.selectName(ctx, file.AID); err != nil {
			return simfile.FCI{}, err
		}
	} else {
		if _, err := r.selectID(ctx, masterFile); err != nil {
			return simfile.FCI{}, err
		}
	}

	var (
		info simfile.FCI
		err  error
	)
	if len(file.Path) == 2 {
		info, err = r.selectID(ctx, file.Path)
	} else {
		info, err = r.selectPath(ctx, file.Path)
	}
	if err != nil {
		return simfile.FCI{}, err
	}

	r.selected = selection{
		valid: true,
		file: usimcard.FileRef{
			AID:  slices.Clone(file.AID),
			Path: slices.Clone(file.Path),
		},
		info: info,
	}
	return info, nil
}

func (r *Reader) selectID(ctx context.Context, id []byte) (simfile.FCI, error) {
	return r.decodeFCI(ctx, command.SelectID{ID: slices.Clone(id)})
}

func (r *Reader) selectName(ctx context.Context, name []byte) (simfile.FCI, error) {
	return r.decodeFCI(ctx, command.SelectName{Name: slices.Clone(name)})
}

func (r *Reader) selectPath(ctx context.Context, path []byte) (simfile.FCI, error) {
	return r.decodeFCI(ctx, command.SelectPath{Path: slices.Clone(path)})
}

func (r *Reader) decodeFCI(ctx context.Context, cmd encoding.BinaryMarshaler) (simfile.FCI, error) {
	raw, err := r.transmitCommand(ctx, cmd)
	if err != nil {
		return simfile.FCI{}, err
	}

	var info simfile.FCI
	if err := info.UnmarshalBinary(raw.Data()); err != nil {
		return simfile.FCI{}, err
	}
	return info, nil
}

func (r *Reader) readBinaryRaw(ctx context.Context, offset, length uint16) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}

	data := make([]byte, 0, length)
	for length > 0 {
		chunk := min(length, uint16(0xFF))
		raw, err := r.transmitCommand(ctx, command.BinaryRead{
			Offset: offset,
			Length: byte(chunk),
		})
		if err != nil {
			return nil, err
		}
		data = append(data, raw.Data()...)
		offset += chunk
		length -= chunk
	}
	return data, nil
}

func (r *Reader) readRecordRaw(ctx context.Context, record, length uint16) ([]byte, error) {
	if record == 0 {
		return nil, errors.New("reading record file: record number is zero")
	}
	if length > 0xFF {
		return nil, fmt.Errorf("reading record file: record length %d exceeds APDU limit", length)
	}

	raw, err := r.transmitCommand(ctx, command.RecordRead{
		Record: byte(record),
		Length: byte(length),
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(raw.Data()), nil
}

func (r *Reader) transmitCommand(ctx context.Context, cmd encoding.BinaryMarshaler) (apdu.Response, error) {
	req, err := cmd.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return r.transmit(ctx, req)
}

func (r *Reader) transmitEnvelopeCommand(ctx context.Context, cmd encoding.BinaryMarshaler) (apdu.Response, error) {
	req, err := cmd.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return r.transmitEnvelope(ctx, req)
}

func (r *Reader) transmit(ctx context.Context, req []byte) (apdu.Response, error) {
	raw, err := r.tx.Transmit(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := apdu.Response(raw)
	if len(resp) < 2 {
		return nil, apdu.ErrMalformedResponse
	}

	data := append([]byte(nil), resp.Data()...)
	for resp.HasMore() {
		length := resp.SW2()
		req, err := (apdu.Request{
			CLA: 0x00,
			INS: 0xC0,
			P1:  0x00,
			P2:  0x00,
			Le:  &length,
		}).MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building GET RESPONSE APDU: %w", err)
		}
		follow, err := r.tx.Transmit(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}
		resp = apdu.Response(follow)
		if len(resp) < 2 {
			return nil, apdu.ErrMalformedResponse
		}
		data = append(data, resp.Data()...)
	}

	if !resp.OK() {
		return nil, apdu.StatusError{SW: resp.SW()}
	}
	return append(data, byte(resp.SW()>>8), byte(resp.SW())), nil
}

func (r *Reader) transmitEnvelope(ctx context.Context, req []byte) (apdu.Response, error) {
	raw, err := r.tx.Transmit(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := apdu.Response(raw)
	if len(resp) < 2 {
		return nil, apdu.ErrMalformedResponse
	}

	data := append([]byte(nil), resp.Data()...)
	for resp.HasMore() {
		length := resp.SW2()
		req, err := (apdu.Request{CLA: 0x00, INS: 0xC0, P1: 0x00, P2: 0x00, Le: &length}).MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building GET RESPONSE APDU: %w", err)
		}
		follow, err := r.tx.Transmit(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}
		resp = apdu.Response(follow)
		if len(resp) < 2 {
			return nil, apdu.ErrMalformedResponse
		}
		data = append(data, resp.Data()...)
	}

	if resp.SW1() != 0x90 && resp.SW1() != 0x91 {
		return nil, apdu.StatusError{SW: resp.SW()}
	}
	return append(data, byte(resp.SW()>>8), byte(resp.SW())), nil
}

func (s selection) matches(file usimcard.FileRef) bool {
	return s.valid && bytes.Equal(s.file.AID, file.AID) && bytes.Equal(s.file.Path, file.Path)
}
