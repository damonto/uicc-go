package uim

import (
	"bytes"
	"encoding"
	"strings"
	"testing"
)

func TestAuthenticateRequestImplementsStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = AuthenticateRequest{}
	var _ encoding.BinaryUnmarshaler = (*AuthenticateRequest)(nil)
}

func TestAuthenticateRequestMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		req     AuthenticateRequest
		want    []byte
		wantErr string
	}{
		{
			name: "3g auth",
			req: AuthenticateRequest{
				Context: AuthContext3G,
				Rand:    bytes.Repeat([]byte{0x01}, 16),
				AUTN:    bytes.Repeat([]byte{0x02}, 16),
			},
			want: append(
				append(
					[]byte{byte(AuthContext3G), 0x22, 0x00, 0x10},
					bytes.Repeat([]byte{0x01}, 16)...,
				),
				append([]byte{0x10}, bytes.Repeat([]byte{0x02}, 16)...)...,
			),
		},
		{
			name: "reject long rand",
			req: AuthenticateRequest{
				Rand: bytes.Repeat([]byte{0x01}, 256),
			},
			wantErr: "rand length 256 exceeds 255",
		},
		{
			name: "reject long autn",
			req: AuthenticateRequest{
				AUTN: bytes.Repeat([]byte{0x02}, 256),
			},
			wantErr: "autn length 256 exceeds 255",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestAuthenticateRequestUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    AuthenticateRequest
		wantErr bool
	}{
		{
			name: "round trip",
			data: []byte{
				byte(AuthContextIMSAKA),
				0x06, 0x00,
				0x02, 0x01, 0x02,
				0x02, 0x03, 0x04,
			},
			want: AuthenticateRequest{
				Context: AuthContextIMSAKA,
				Rand:    []byte{0x01, 0x02},
				AUTN:    []byte{0x03, 0x04},
			},
		},
		{
			name:    "short header",
			data:    []byte{byte(AuthContext3G), 0x00},
			wantErr: true,
		},
		{
			name:    "body length mismatch",
			data:    []byte{byte(AuthContext3G), 0x02, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "rand truncated",
			data:    []byte{byte(AuthContext3G), 0x02, 0x00, 0x02, 0x01},
			wantErr: true,
		},
		{
			name:    "autn length mismatch",
			data:    []byte{byte(AuthContext3G), 0x03, 0x00, 0x00, 0x02, 0x01},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got AuthenticateRequest
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.Context != tt.want.Context || !bytes.Equal(got.Rand, tt.want.Rand) || !bytes.Equal(got.AUTN, tt.want.AUTN) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
			got.Rand[0] = 0xFF
			if bytes.Equal(got.Rand, tt.data[4:6]) {
				t.Fatal("UnmarshalBinary() kept rand backing array")
			}
		})
	}
}

func TestAuthenticateRequestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  AuthenticateRequest
	}{
		{
			name: "3g auth",
			req: AuthenticateRequest{
				Context: AuthContext3G,
				Rand:    []byte{0x01, 0x02, 0x03},
				AUTN:    []byte{0x04, 0x05},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.req.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			var got AuthenticateRequest
			if err := got.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.Context != tt.req.Context || !bytes.Equal(got.Rand, tt.req.Rand) || !bytes.Equal(got.AUTN, tt.req.AUTN) {
				t.Fatalf("round trip = %+v, want %+v", got, tt.req)
			}
		})
	}
}

func TestAuthenticateRequestUnmarshalDoesNotSetSession(t *testing.T) {
	var req AuthenticateRequest
	err := req.UnmarshalBinary([]byte{byte(AuthContext3G), 0x02, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if req.Session != 0 || len(req.AID) != 0 {
		t.Fatalf("UnmarshalBinary() session=%v aid=%X, want zero values", req.Session, req.AID)
	}
}
