package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/simfile"
)

type FindAID struct {
	Label    string
	Prefix   []byte
	NotFound error
}

type App struct {
	Reader usimcard.Reader
	AID    []byte
}

func (c FindAID) Run(ctx context.Context, r usimcard.Reader) ([]byte, error) {
	apps, err := r.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	for _, app := range apps {
		if len(app.AID) == 0 {
			continue
		}
		if strings.EqualFold(app.Label, c.Label) || (len(c.Prefix) > 0 && bytes.HasPrefix(app.AID, c.Prefix)) {
			return slices.Clone(app.AID), nil
		}
	}

	if c.NotFound != nil {
		return nil, c.NotFound
	}
	return nil, errors.New("application not found")
}

func (a App) Transparent(ctx context.Context, path []byte, action string) ([]byte, error) {
	file := a.file(path)
	attrs, err := a.fileStructure(ctx, file, action, simfile.StructureTransparent)
	if err != nil {
		return nil, err
	}

	data, err := a.Reader.ReadTransparent(ctx, usimcard.TransparentRead{
		File:   file,
		Length: attrs.FileSize,
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(data), nil
}

func (a App) Text(ctx context.Context, path []byte, action string) (simfile.Text, error) {
	data, err := a.Transparent(ctx, path, action)
	if err != nil {
		return "", err
	}

	var text simfile.Text
	if err := text.UnmarshalBinary(data); err != nil {
		return "", err
	}
	return text, nil
}

func (a App) LinearFixed(ctx context.Context, path []byte, action string) ([][]byte, error) {
	file := a.file(path)
	attrs, err := a.fileStructure(ctx, file, action, simfile.StructureLinearFixed)
	if err != nil {
		return nil, err
	}
	if attrs.RecordCount == 0 {
		return nil, fmt.Errorf("%s: file has no records", action)
	}

	records := make([][]byte, 0, attrs.RecordCount)
	for recordID := uint16(1); recordID <= attrs.RecordCount; recordID++ {
		record, err := a.Reader.ReadRecord(ctx, usimcard.RecordRead{
			File:   file,
			Record: recordID,
			Length: attrs.RecordSize,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, slices.Clone(record))
	}
	return records, nil
}

func (a App) FirstText(ctx context.Context, path []byte, action string) (simfile.Text, error) {
	file := a.file(path)
	attrs, err := a.fileStructure(ctx, file, action, simfile.StructureLinearFixed)
	if err != nil {
		return "", err
	}
	if attrs.RecordCount == 0 {
		return "", fmt.Errorf("%s: file has no records", action)
	}

	for recordID := uint16(1); recordID <= attrs.RecordCount; recordID++ {
		record, err := a.Reader.ReadRecord(ctx, usimcard.RecordRead{
			File:   file,
			Record: recordID,
			Length: attrs.RecordSize,
		})
		if err != nil {
			return "", err
		}

		var value simfile.Text
		if err := value.UnmarshalBinary(record); err != nil {
			continue
		}
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s: no populated record", action)
}

func (a App) file(path []byte) usimcard.FileRef {
	return usimcard.FileRef{
		AID:  slices.Clone(a.AID),
		Path: slices.Clone(path),
	}
}

func (a App) fileStructure(ctx context.Context, file usimcard.FileRef, action string, want simfile.FileStructure) (usimcard.FileAttributes, error) {
	attrs, err := a.Reader.FileAttributes(ctx, file)
	if err != nil {
		return usimcard.FileAttributes{}, err
	}
	if attrs.FileStructure != want {
		return usimcard.FileAttributes{}, fmt.Errorf("%s: unexpected file structure", action)
	}
	return attrs, nil
}
