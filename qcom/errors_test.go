package qcom

import (
	"encoding/binary"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestQMIErrorFallbackIncludesCode(t *testing.T) {
	err := QMIError(65000)

	if got, want := err.Error(), "QMI error 65000"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, QMIError(65000)) {
		t.Fatal("QMIError should remain comparable through errors.Is")
	}
}

func TestQMIErrorInvalidArgumentHasText(t *testing.T) {
	if got, want := QMIErrorInvalidArgument.Error(), "Invalid argument"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestQMIErrorLaterCodesHaveText(t *testing.T) {
	tests := map[QMIError]string{
		QMIErrorInvalidIndex:               "Invalid index",
		QMIErrorOperationInProgress:        "Operation in progress",
		QMIErrorCatEnvelopeCommandFailed:   "CAT envelope command failed",
		QMIErrorFwUpdateDiscontinuousFrame: "Firmware update discontinuous frame",
	}

	for code, want := range tests {
		if got := code.Error(); got != want {
			t.Fatalf("%d Error() = %q, want %q", code, got, want)
		}
	}
}

func TestQMIErrorTextCoversDeclaredErrors(t *testing.T) {
	data, err := os.ReadFile("errors.go")
	if err != nil {
		t.Fatalf("read errors.go: %v", err)
	}

	constRe := regexp.MustCompile(`(?m)^\s*(QMIError[A-Za-z0-9]+)\s+QMIError\s*=`)
	mapRe := regexp.MustCompile(`(?m)^\s*(QMIError[A-Za-z0-9]+):\s*"`)

	mapped := make(map[string]bool)
	for _, match := range mapRe.FindAllStringSubmatch(string(data), -1) {
		mapped[match[1]] = true
	}

	for _, match := range constRe.FindAllStringSubmatch(string(data), -1) {
		if !mapped[match[1]] {
			t.Fatalf("%s is declared but not mapped to text", match[1])
		}
	}
}

func TestResultError(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr error
	}{
		{
			name: "success",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x02, []byte{0x00, 0x00, 0x00, 0x00}),
			},
		},
		{
			name: "failure",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x02, binary.LittleEndian.AppendUint16([]byte{0x01, 0x00}, uint16(QMIErrorNotSupported))),
			},
			wantErr: QMIErrorNotSupported,
		},
		{
			name:    "missing",
			tlvs:    nil,
			wantErr: errNoResultTLV,
		},
		{
			name: "truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x02, []byte{0x00}),
			},
			wantErr: errShortResultTLV,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ResultError(tt.tlvs)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ResultError() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ResultError() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
