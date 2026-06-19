package usim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	qc "github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/uim"
	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/simfile"
)

var (
	qcomUSIMAIDPrefix = []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	qcomISIMAIDPrefix = []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	qcomCurrentADF    = []byte{0x7F, 0xFF}
)

type QCOM struct {
	reader *uim.Reader
}

func NewQCOM(reader *uim.Reader) (*QCOM, error) {
	if reader == nil {
		return nil, errors.New("creating QCOM adapter: reader is nil")
	}
	return &QCOM{reader: reader}, nil
}

func (r *QCOM) ListApplications(ctx context.Context) ([]usimcard.Application, error) {
	attrs, err := r.FileAttributes(ctx, usimcard.FileRef{Path: efDirFile})
	if err != nil {
		return nil, fmt.Errorf("reading EF_DIR: %w", err)
	}
	if attrs.FileStructure != simfile.StructureLinearFixed {
		return nil, errors.New("reading EF_DIR: unexpected file structure")
	}

	apps := make([]usimcard.Application, 0, attrs.RecordCount)
	for recordID := uint16(1); recordID <= attrs.RecordCount; recordID++ {
		record, err := r.ReadRecord(ctx, usimcard.RecordRead{
			File:   usimcard.FileRef{Path: efDirFile},
			Record: recordID,
			Length: attrs.RecordSize,
		})
		if err != nil {
			return nil, fmt.Errorf("reading EF_DIR record %d: %w", recordID, err)
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

func (r *QCOM) FileAttributes(ctx context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
	attrs, err := r.fileAttributes(ctx, file)
	if err != nil {
		return usimcard.FileAttributes{}, err
	}
	return usimcard.FileAttributes{
		FileStructure: simfile.FileStructure(attrs.FileStructure),
		FileType:      simfile.FileType(attrs.FileType),
		RecordSize:    attrs.RecordSize,
		RecordCount:   attrs.RecordCount,
		FileSize:      attrs.FileSize,
	}, nil
}

func (r *QCOM) ReadTransparent(ctx context.Context, req usimcard.TransparentRead) ([]byte, error) {
	files, err := r.files(req.File)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, file := range files {
		data, err := r.reader.ReadTransparent(ctx, uim.TransparentRead{
			File:   file,
			Offset: req.Offset,
			Length: req.Length,
		})
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) ReadRecord(ctx context.Context, req usimcard.RecordRead) ([]byte, error) {
	files, err := r.files(req.File)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, file := range files {
		data, err := r.reader.ReadRecord(ctx, uim.RecordRead{
			File:   file,
			Record: req.Record,
			Length: req.Length,
		})
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) Authenticate3G(ctx context.Context, req usimcard.AuthenticateRequest) ([]byte, error) {
	requests, err := r.authRequests(req)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, request := range requests {
		data, err := r.reader.Authenticate(ctx, request)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(requests) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) SMSPPDownload(ctx context.Context, req usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error) {
	envelope, err := command.SMSPPDownload{
		ServiceCenterAddress: req.ServiceCenterAddress,
		TPDU:                 slices.Clone(req.TPDU),
	}.Envelope()
	if err != nil {
		return usimcard.SMSPPDownloadResponse{}, fmt.Errorf("building SMS-PP envelope: %w", err)
	}

	resp, err := r.reader.SendEnvelope(ctx, envelope)
	if err != nil {
		return usimcard.SMSPPDownloadResponse{}, err
	}
	return usimcard.SMSPPDownloadResponse{
		SW1:  resp.SW1,
		SW2:  resp.SW2,
		Data: slices.Clone(resp.Data),
	}, nil
}

func (r *QCOM) Close() error {
	return r.reader.Close()
}

func (r *QCOM) fileAttributes(ctx context.Context, file usimcard.FileRef) (uim.FileAttributes, error) {
	files, err := r.files(file)
	if err != nil {
		return uim.FileAttributes{}, err
	}
	var lastErr error
	for i, file := range files {
		attrs, err := r.reader.GetFileAttributes(ctx, file)
		if err == nil {
			return attrs, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return uim.FileAttributes{}, lastErr
}

func (r *QCOM) files(file usimcard.FileRef) ([]uim.File, error) {
	path := qcomFilePath(file)
	if len(file.AID) == 0 {
		return []uim.File{{Session: uim.SessionPrimaryGWProvisioning, Path: path}}, nil
	}
	if hasPrefix(file.AID, qcomISIMAIDPrefix) {
		cardSession, err := qcomCardSession(r.reader.Slot())
		if err != nil {
			return nil, err
		}
		nonProvisioningSession, err := qcomNonProvisioningSession(r.reader.Slot())
		if err != nil {
			return nil, err
		}
		return []uim.File{
			{Session: cardSession, AID: slices.Clone(file.AID), Path: path},
			{Session: nonProvisioningSession, AID: slices.Clone(file.AID), Path: path},
		}, nil
	}
	if hasPrefix(file.AID, qcomUSIMAIDPrefix) {
		return []uim.File{{Session: uim.SessionPrimaryGWProvisioning, Path: path}}, nil
	}
	return []uim.File{{Session: uim.SessionPrimaryGWProvisioning, AID: slices.Clone(file.AID), Path: path}}, nil
}

func (r *QCOM) authRequests(req usimcard.AuthenticateRequest) ([]uim.AuthenticateRequest, error) {
	ctx := uim.AuthContext3G
	if req.EAPAKA || hasPrefix(req.AID, qcomISIMAIDPrefix) {
		ctx = uim.AuthContextIMSAKA
	}
	if len(req.AID) == 0 {
		return []uim.AuthenticateRequest{{
			Session: uim.SessionPrimaryGWProvisioning,
			Context: ctx,
			Rand:    slices.Clone(req.Rand),
			AUTN:    slices.Clone(req.AUTN),
		}}, nil
	}
	if hasPrefix(req.AID, qcomISIMAIDPrefix) {
		cardSession, err := qcomCardSession(r.reader.Slot())
		if err != nil {
			return nil, err
		}
		nonProvisioningSession, err := qcomNonProvisioningSession(r.reader.Slot())
		if err != nil {
			return nil, err
		}
		return []uim.AuthenticateRequest{
			{Session: cardSession, AID: slices.Clone(req.AID), Context: ctx, Rand: slices.Clone(req.Rand), AUTN: slices.Clone(req.AUTN)},
			{Session: nonProvisioningSession, AID: slices.Clone(req.AID), Context: ctx, Rand: slices.Clone(req.Rand), AUTN: slices.Clone(req.AUTN)},
		}, nil
	}
	if hasPrefix(req.AID, qcomUSIMAIDPrefix) {
		return []uim.AuthenticateRequest{{
			Session: uim.SessionPrimaryGWProvisioning,
			Context: ctx,
			Rand:    slices.Clone(req.Rand),
			AUTN:    slices.Clone(req.AUTN),
		}}, nil
	}
	return []uim.AuthenticateRequest{{
		Session: uim.SessionPrimaryGWProvisioning,
		AID:     slices.Clone(req.AID),
		Context: ctx,
		Rand:    slices.Clone(req.Rand),
		AUTN:    slices.Clone(req.AUTN),
	}}, nil
}

func qcomFilePath(file usimcard.FileRef) []byte {
	if len(file.AID) != 0 {
		if hasPrefix(file.AID, qcomUSIMAIDPrefix) {
			return joinBytes(masterFile, qcomCurrentADF, file.Path)
		}
		return slices.Clone(file.Path)
	}
	if hasPrefix(file.Path, masterFile) {
		return slices.Clone(file.Path)
	}
	return joinBytes(masterFile, file.Path)
}

func qcomCardSession(slot uint8) (uim.Session, error) {
	switch slot {
	case 1:
		return uim.SessionCardSlot1, nil
	case 2:
		return uim.SessionCardSlot2, nil
	case 3:
		return uim.SessionCardSlot3, nil
	case 4:
		return uim.SessionCardSlot4, nil
	case 5:
		return uim.SessionCardSlot5, nil
	default:
		return 0, fmt.Errorf("mapping card session: slot %d is out of range", slot)
	}
}

func qcomNonProvisioningSession(slot uint8) (uim.Session, error) {
	switch slot {
	case 1:
		return uim.SessionNonProvisioningSlot1, nil
	case 2:
		return uim.SessionNonProvisioningSlot2, nil
	case 3:
		return uim.SessionNonProvisioningSlot3, nil
	case 4:
		return uim.SessionNonProvisioningSlot4, nil
	case 5:
		return uim.SessionNonProvisioningSlot5, nil
	default:
		return 0, fmt.Errorf("mapping nonprovisioning session: slot %d is out of range", slot)
	}
}

func retryableQCOMSessionError(err error) bool {
	return errors.Is(err, qc.QMIErrorSessionInactive) ||
		errors.Is(err, qc.QMIErrorSessionInvalid) ||
		errors.Is(err, qc.QMIErrorInvalidSessionType) ||
		errors.Is(err, qc.QMIErrorAuthenticationFailed) ||
		errors.Is(err, qc.QMIErrorAccessDenied)
}

func hasPrefix(value, prefix []byte) bool {
	return len(value) >= len(prefix) && slices.Equal(value[:len(prefix)], prefix)
}

func joinBytes(parts ...[]byte) []byte {
	total := 0
	for _, part := range parts {
		total += len(part)
	}

	buf := make([]byte, 0, total)
	for _, part := range parts {
		buf = append(buf, part...)
	}
	return buf
}
