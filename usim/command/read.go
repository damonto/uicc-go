package command

import "github.com/damonto/uicc-go/apdu"

type BinaryRead struct {
	Length byte
	Offset uint16
}

type RecordRead struct {
	Record byte
	Length byte
}

func (c BinaryRead) MarshalBinary() ([]byte, error) {
	return apdu.Request{
		CLA: 0x00,
		INS: 0xB0,
		P1:  byte(c.Offset >> 8),
		P2:  byte(c.Offset),
		Le:  &c.Length,
	}.MarshalBinary()
}

func (c RecordRead) MarshalBinary() ([]byte, error) {
	return apdu.Request{
		CLA: 0x00,
		INS: 0xB2,
		P1:  c.Record,
		P2:  0x04,
		Le:  &c.Length,
	}.MarshalBinary()
}
