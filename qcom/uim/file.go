package uim

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func (r *Reader) transparentResponse(
	ctx context.Context,
	file File,
	offset uint16,
	length uint16,
) (qcom.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qcom.Response{}, err
	}

	info := joinBytes(
		binary.LittleEndian.AppendUint16(nil, offset),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	resp, err := r.request(ctx, qcom.MessageReadTransparent, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, info),
	})
	if err != nil {
		return qcom.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qcom.Response{}, err
	}
	return resp, nil
}

func (r *Reader) recordResponse(
	ctx context.Context,
	file File,
	record uint16,
	length uint16,
) (qcom.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qcom.Response{}, err
	}

	recordValue := joinBytes(
		binary.LittleEndian.AppendUint16(nil, record),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	resp, err := r.request(ctx, qcom.MessageReadRecord, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, recordValue),
	})
	if err != nil {
		return qcom.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qcom.Response{}, err
	}
	return resp, nil
}

func (r *Reader) fileAttributesResponse(
	ctx context.Context,
	file File,
) (qcom.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qcom.Response{}, err
	}

	resp, err := r.request(ctx, qcom.MessageGetFileAttributes, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
	})
	if err != nil {
		return qcom.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qcom.Response{}, err
	}
	return resp, nil
}

func (r *Reader) authenticateResponse(
	ctx context.Context,
	req AuthenticateRequest,
) (qcom.Response, error) {
	value, err := req.MarshalBinary()
	if err != nil {
		return qcom.Response{}, err
	}

	resp, err := r.request(ctx, qcom.MessageAuthenticate, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(req.Session, req.AID)),
		tlv.Bytes(0x02, value),
	})
	if err != nil {
		return qcom.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qcom.Response{}, err
	}
	return resp, nil
}

func decodeReaderFileAttributes(resp qcom.Response) (FileAttributes, error) {
	value, ok := tlv.Value(resp.TLVs, 0x11)
	if !ok {
		return FileAttributes{}, errors.New("reading file attributes: attributes TLV missing")
	}

	attrs, err := decodeFileAttributes(value)
	if err != nil {
		return FileAttributes{}, err
	}

	return FileAttributes{
		FileSize:      attrs.FileSize,
		RecordSize:    attrs.RecordSize,
		RecordCount:   attrs.RecordCount,
		FileType:      fileTypeToSIMFileType(attrs.FileType),
		FileStructure: fileTypeToSIMFileStructure(attrs.FileType),
	}, nil
}

func fileTypeToSIMFileStructure(fileType QMIFileType) FileStructure {
	switch fileType {
	case QMIFileTypeTransparent:
		return FileStructureTransparent
	case QMIFileTypeLinearFixed:
		return FileStructureLinearFixed
	default:
		return 0
	}
}

func fileTypeToSIMFileType(fileType QMIFileType) FileType {
	switch fileType {
	case QMIFileTypeTransparent, QMIFileTypeCyclic, QMIFileTypeLinearFixed:
		return FileTypeWorkingEF
	case QMIFileTypeDedicated, QMIFileTypeMaster:
		return FileTypeDFOrADF
	default:
		return FileType(fileType)
	}
}
