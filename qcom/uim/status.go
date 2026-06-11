package uim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

type RawFileAttributes struct {
	FileSize    uint16
	FileID      uint16
	FileType    QMIFileType
	RecordSize  uint16
	RecordCount uint16
	Raw         []byte
}

type SlotStatus struct {
	ActiveSlot uint8
	Slots      []Slot
}

type Slot struct {
	PhysicalCardStatus PhysicalCardState
	PhysicalSlotStatus SlotState
	LogicalSlot        uint8
	ICCID              []byte
}

type CardStatus struct {
	IndexGWPrimary   uint16
	Index1XPrimary   uint16
	IndexGWSecondary uint16
	Index1XSecondary uint16
	Cards            []Card
}

type Card struct {
	State        CardState
	UPINState    PINState
	UPINRetries  byte
	UPUKRetries  byte
	ErrorCode    CardError
	Applications []CardApplication
}

type CardApplication struct {
	Type                          ApplicationType
	State                         ApplicationState
	PersonalizationState          PersonalizationState
	PersonalizationFeature        PersonalizationFeature
	PersonalizationRetries        byte
	PersonalizationUnblockRetries byte
	AID                           []byte
	UPINReplacesPIN1              byte
	PIN1State                     PINState
	PIN1Retries                   byte
	PUK1Retries                   byte
	PIN2State                     PINState
	PIN2Retries                   byte
	PUK2Retries                   byte
}

func decodeSlotStatus(resp qcom.Response) (SlotStatus, error) {
	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return SlotStatus{}, errors.New("reading slot status: status TLV missing")
	}

	payload := newPayloadReader(value)
	count := payload.Uint8()
	if err := payload.Err(); err != nil {
		return SlotStatus{}, fmt.Errorf("reading slot status: %w", err)
	}

	status := SlotStatus{
		Slots: make([]Slot, 0, count),
	}
	for i := range count {
		slot := Slot{
			PhysicalCardStatus: PhysicalCardState(payload.Uint32()),
			PhysicalSlotStatus: SlotState(payload.Uint32()),
			LogicalSlot:        payload.Uint8(),
			ICCID:              payload.Bytes8(),
		}
		if err := payload.Err(); err != nil {
			return SlotStatus{}, fmt.Errorf("reading slot status: %w", err)
		}

		status.Slots = append(status.Slots, slot)
		if slot.PhysicalSlotStatus == SlotStateActive {
			status.ActiveSlot = uint8(i + 1)
		}
	}
	return status, nil
}

func decodeCardStatus(resp qcom.Response) (CardStatus, error) {
	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return CardStatus{}, errors.New("reading card status: status TLV missing")
	}

	payload := newPayloadReader(value)
	status := CardStatus{}
	status.IndexGWPrimary = payload.Uint16()
	status.Index1XPrimary = payload.Uint16()
	status.IndexGWSecondary = payload.Uint16()
	status.Index1XSecondary = payload.Uint16()

	cardCount := payload.Uint8()
	if err := payload.Err(); err != nil {
		return CardStatus{}, fmt.Errorf("reading card status: %w", err)
	}

	status.Cards = make([]Card, 0, cardCount)
	for range cardCount {
		entry := Card{
			State:       CardState(payload.Uint8()),
			UPINState:   PINState(payload.Uint8()),
			UPINRetries: payload.Uint8(),
			UPUKRetries: payload.Uint8(),
			ErrorCode:   CardError(payload.Uint8()),
		}
		appCount := payload.Uint8()
		if err := payload.Err(); err != nil {
			return CardStatus{}, fmt.Errorf("reading card status: %w", err)
		}

		entry.Applications = make([]CardApplication, 0, appCount)
		for range appCount {
			app := CardApplication{
				Type:                          ApplicationType(payload.Uint8()),
				State:                         ApplicationState(payload.Uint8()),
				PersonalizationState:          PersonalizationState(payload.Uint8()),
				PersonalizationFeature:        PersonalizationFeature(payload.Uint8()),
				PersonalizationRetries:        payload.Uint8(),
				PersonalizationUnblockRetries: payload.Uint8(),
			}
			app.AID = payload.Bytes8()
			app.UPINReplacesPIN1 = payload.Uint8()
			app.PIN1State = PINState(payload.Uint8())
			app.PIN1Retries = payload.Uint8()
			app.PUK1Retries = payload.Uint8()
			app.PIN2State = PINState(payload.Uint8())
			app.PIN2Retries = payload.Uint8()
			app.PUK2Retries = payload.Uint8()
			if err := payload.Err(); err != nil {
				return CardStatus{}, fmt.Errorf("reading card status: %w", err)
			}

			entry.Applications = append(entry.Applications, app)
		}
		status.Cards = append(status.Cards, entry)
	}
	return status, nil
}

func (s CardStatus) Ready() bool {
	for _, card := range s.Cards {
		if card.State != CardStatePresent {
			continue
		}
		for _, app := range card.Applications {
			if app.Type == ApplicationTypeUSIM && app.State == ApplicationStateReady {
				return true
			}
		}
	}
	return false
}

func decodeFileAttributes(data []byte) (RawFileAttributes, error) {
	if len(data) < 9 {
		return RawFileAttributes{}, errors.New("reading file attributes: attributes payload is truncated")
	}

	attrs := RawFileAttributes{
		FileSize:    binary.LittleEndian.Uint16(data[:2]),
		FileID:      binary.LittleEndian.Uint16(data[2:4]),
		FileType:    QMIFileType(data[4]),
		RecordSize:  binary.LittleEndian.Uint16(data[5:7]),
		RecordCount: binary.LittleEndian.Uint16(data[7:9]),
	}
	if len(data) < 26 {
		return attrs, nil
	}

	rawLength := int(binary.LittleEndian.Uint16(data[24:26]))
	if len(data) < 26+rawLength {
		return RawFileAttributes{}, errors.New("reading file attributes: raw data is truncated")
	}
	attrs.Raw = slices.Clone(data[26 : 26+rawLength])
	return attrs, nil
}

type payloadReader struct {
	r   *bytes.Reader
	err error
}

func newPayloadReader(data []byte) *payloadReader {
	return &payloadReader{r: bytes.NewReader(data)}
}

func (r *payloadReader) Uint8() uint8 {
	data := r.Bytes(1)
	if r.err != nil {
		return 0
	}
	return data[0]
}

func (r *payloadReader) Uint16() uint16 {
	data := r.Bytes(2)
	if r.err != nil {
		return 0
	}
	return binary.LittleEndian.Uint16(data)
}

func (r *payloadReader) Uint32() uint32 {
	data := r.Bytes(4)
	if r.err != nil {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

func (r *payloadReader) Bytes8() []byte {
	return r.Bytes(int(r.Uint8()))
}

func (r *payloadReader) Bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if r.r.Len() < n {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r.r, data); err != nil {
		r.err = err
		return nil
	}
	return data
}

func (r *payloadReader) Err() error {
	return r.err
}
