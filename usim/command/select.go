package command

import (
	"slices"

	"github.com/damonto/uicc-go/apdu"
)

type SelectID struct {
	ID []byte
}

type SelectName struct {
	Name []byte
}

type SelectPath struct {
	Path []byte
}

func (c SelectID) MarshalBinary() ([]byte, error) {
	return selectRequest(0x00, c.ID)
}

func (c SelectName) MarshalBinary() ([]byte, error) {
	return selectRequest(0x04, c.Name)
}

func (c SelectPath) MarshalBinary() ([]byte, error) {
	return selectRequest(0x08, c.Path)
}

func selectRequest(p1 byte, value []byte) ([]byte, error) {
	return apdu.Request{
		CLA:  0x00,
		INS:  0xA4,
		P1:   p1,
		P2:   0x04,
		Data: slices.Clone(value),
	}.MarshalBinary()
}
