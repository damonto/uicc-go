package qcom

// QMUX header constants
const (
	QMUXHeaderIfType             = 0x01
	QMUXHeaderControlFlagRequest = 0x00

	QMUXIfType             = QMUXHeaderIfType
	QMUXControlFlagRequest = QMUXHeaderControlFlagRequest
)

const (
	MaxEncodedMessageLength = 0xffff

	QMUXHeaderLength         = 1 + 5
	QMIControlHeaderLength   = 6
	QMIServiceHeaderLength   = 7
	QRTRInternalHeaderLength = 1 + 5

	MaxQMUXControlTLVLength = MaxEncodedMessageLength - QMUXHeaderLength - QMIControlHeaderLength
	MaxQMUXServiceTLVLength = MaxEncodedMessageLength - QMUXHeaderLength - QMIServiceHeaderLength
	MaxQRTRServiceTLVLength = MaxEncodedMessageLength - QRTRInternalHeaderLength - QMIServiceHeaderLength
	MaxQRTRQMIMessageLength = QMIServiceHeaderLength + MaxQRTRServiceTLVLength
)
