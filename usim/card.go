package usim

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/simfile"
)

var (
	fileEFICCID   = mustHex("2FE2")
	fileEFIMSI    = mustHex("6F07")
	fileEFAD      = mustHex("6FAD")
	fileEFSMSP    = mustHex("6F42")
	fileEFGID1    = mustHex("6F3E")
	pathEFPSISMSC = mustHex("7F106FE5")
	fileEFIMPI    = mustHex("6F02")
	fileEFDomain  = mustHex("6F03")
	fileEFIMPU    = mustHex("6F04")
	errNoUSIMAID  = errors.New("USIM application not found")
	errNoISIMAID  = errors.New("ISIM application not found")
)

type Card struct {
	tx     usimcard.Reader
	logger *slog.Logger

	aid             []byte
	isimAID         []byte
	iccid           string
	imsi            string
	mcc             string
	mnc             string
	mncLength       int
	gid1            string
	serviceCenter   ServiceCenter
	privateIdentity string
	publicIdentity  string
	homeDomain      string
	akaState        AKAIdentityState
}

func (u *Card) ICCID() string                              { return u.iccid }
func (u *Card) IMSI() string                               { return u.imsi }
func (u *Card) MCC() string                                { return u.mcc }
func (u *Card) MNC() string                                { return u.mnc }
func (u *Card) MNCLength() int                             { return u.mncLength }
func (u *Card) GID1() string                               { return u.gid1 }
func (u *Card) SMSC() string                               { return u.serviceCenter.Address }
func (u *Card) ServiceCenter() ServiceCenter               { return u.serviceCenter }
func (u *Card) PrivateIdentity() string                    { return u.privateIdentity }
func (u *Card) PublicIdentity() string                     { return u.publicIdentity }
func (u *Card) HomeDomain() string                         { return u.homeDomain }
func (u *Card) AKAIdentityState() AKAIdentityState         { return u.akaState.clone() }
func (u *Card) SetAKAIdentityState(state AKAIdentityState) { u.akaState = state.clone() }

func New(ctx context.Context, tx usimcard.Reader, loggers ...*slog.Logger) (*Card, error) {
	if tx == nil {
		return nil, errors.New("creating USIM: reader is nil")
	}
	if len(loggers) > 1 {
		return nil, errors.New("creating USIM: multiple loggers configured")
	}

	logger := slog.Default()
	if len(loggers) == 1 && loggers[0] != nil {
		logger = loggers[0]
	}

	card := &Card{
		tx:     tx,
		logger: logger.With("component", "usim"),
	}
	if err := card.load(ctx); err != nil {
		return nil, err
	}
	return card, nil
}

func (u *Card) Close() error {
	return u.tx.Close()
}

func (u *Card) AKA(ctx context.Context, rand, autn []byte) (AKAResult, error) {
	if len(rand) != 16 || len(autn) != 16 {
		return AKAResult{}, errors.New("authenticating USIM: rand and autn must be 16 bytes")
	}

	result, err := u.authenticate3G(ctx, u.aid, rand, autn, false)
	if err == nil || len(u.isimAID) == 0 {
		return result, err
	}

	result, isimErr := u.authenticate3G(ctx, u.isimAID, rand, autn, false)
	if isimErr == nil {
		return result, nil
	}
	u.logger.Debug("ISIM AKA fallback failed", "err", isimErr)
	return AKAResult{}, err
}

func (u *Card) EAPAKA(ctx context.Context, rand, autn []byte) (AKAResult, error) {
	if len(rand) != 16 || len(autn) != 16 {
		return AKAResult{}, errors.New("authenticating USIM: rand and autn must be 16 bytes")
	}
	result, err := u.authenticate3G(ctx, u.aid, rand, autn, true)
	if err == nil {
		return result, err
	}
	if len(u.isimAID) != 0 {
		result, isimErr := u.authenticate3G(ctx, u.isimAID, rand, autn, true)
		if isimErr == nil {
			return result, nil
		}
	}
	return u.AKA(ctx, rand, autn)
}

func (u *Card) authenticate3G(ctx context.Context, aid, rand, autn []byte, eapAKA bool) (AKAResult, error) {
	payload, err := u.tx.Authenticate3G(ctx, usimcard.AuthenticateRequest{
		AID:    aid,
		Rand:   rand,
		AUTN:   autn,
		EAPAKA: eapAKA,
	})
	if err != nil {
		return AKAResult{}, fmt.Errorf("authenticating USIM: %w", err)
	}
	var result command.Authenticate3GResult
	if err := result.UnmarshalBinary(payload); err != nil {
		return AKAResult{}, fmt.Errorf("authenticating USIM: %w", err)
	}
	return AKAResult{
		RES:    result.RES,
		CK:     result.CK,
		IK:     result.IK,
		AUTS:   result.AUTS,
		Reject: result.Reject,
	}, nil
}

func (u *Card) SMSPPDownload(ctx context.Context, req usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error) {
	resp, err := u.tx.SMSPPDownload(ctx, req)
	if err != nil {
		return usimcard.SMSPPDownloadResponse{}, fmt.Errorf("running SMS-PP data download: %w", err)
	}
	return resp, nil
}

func (u *Card) load(ctx context.Context) error {
	aid, err := command.FindAID{
		Label:    "USIM",
		Prefix:   []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
		NotFound: errNoUSIMAID,
	}.Run(ctx, u.tx)
	if err != nil {
		return fmt.Errorf("loading USIM: %w", err)
	}
	u.aid = aid
	rootApp := command.App{Reader: u.tx}
	usimApp := command.App{Reader: u.tx, AID: u.aid}

	iccidData, err := rootApp.Transparent(ctx, fileEFICCID, "reading EF_ICCID")
	if err != nil {
		return fmt.Errorf("loading ICCID: %w", err)
	}
	var iccid simfile.ICCID
	if err := iccid.UnmarshalBinary(iccidData); err != nil {
		return fmt.Errorf("loading ICCID: %w", err)
	}
	u.iccid = iccid.String()

	imsiData, err := usimApp.Transparent(ctx, fileEFIMSI, "reading EF_IMSI")
	if err != nil {
		return fmt.Errorf("loading IMSI: %w", err)
	}
	var imsi simfile.IMSI
	if err := imsi.UnmarshalBinary(imsiData); err != nil {
		return fmt.Errorf("loading IMSI: %w", err)
	}
	adData, err := usimApp.Transparent(ctx, fileEFAD, "reading EF_AD")
	if err != nil {
		return fmt.Errorf("loading EF_AD: %w", err)
	}
	var ad simfile.AdministrativeData
	if err := ad.UnmarshalBinary(adData); err != nil {
		return fmt.Errorf("loading EF_AD: %w", err)
	}
	mnc, err := formatMNC(imsi.Digits, ad.MNCLength)
	if err != nil {
		return fmt.Errorf("loading IMSI: %w", err)
	}
	u.imsi = imsi.Digits
	u.mcc = imsi.MCC
	u.mnc = mnc
	u.mncLength = ad.MNCLength

	if gid1, err := usimApp.Transparent(ctx, fileEFGID1, "reading EF_GID1"); err == nil {
		u.gid1 = strings.ToUpper(hex.EncodeToString(gid1))
	} else {
		u.logger.Debug("reading EF_GID1 from USIM failed", "err", err)
	}

	records, err := usimApp.LinearFixed(ctx, fileEFSMSP, "reading EF_SMSP")
	if err == nil {
		if smsc, err := firstSMSC(records); err == nil {
			u.serviceCenter.Address = smsc.String()
		}
	}
	if psi, err := usimApp.FirstText(ctx, pathEFPSISMSC, "reading EFPSISMSC"); err == nil {
		u.serviceCenter.PSI = psi.String()
	} else {
		u.logger.Debug("reading EFPSISMSC from USIM failed", "err", err)
	}
	if err := u.loadISIM(ctx); err != nil {
		u.logger.Debug("skipping ISIM load", "err", err)
	}

	u.logger.Debug(
		"loaded USIM card",
		"iccid", u.iccid,
		"imsi", u.imsi,
		"mcc", u.mcc,
		"mnc", u.mnc,
		"smsc", u.serviceCenter.Address,
		"smsc_psi", u.serviceCenter.PSI,
		"private_identity", u.privateIdentity,
		"public_identity", u.publicIdentity,
		"home_domain", u.homeDomain,
	)

	return nil
}

func (u *Card) loadISIM(ctx context.Context) error {
	aid, err := command.FindAID{
		Label:    "ISIM",
		Prefix:   []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04},
		NotFound: errNoISIMAID,
	}.Run(ctx, u.tx)
	if err != nil {
		return err
	}
	isimApp := command.App{Reader: u.tx, AID: aid}

	u.logger.Debug("found ISIM AID", "aid", fmt.Sprintf("%X", aid))
	u.isimAID = aid
	impi, err := isimApp.Text(ctx, fileEFIMPI, "reading EF_IMPI")
	if err != nil {
		return fmt.Errorf("reading EF_IMPI: %w", err)
	}
	privateIdentity := impi.String()
	var homeDomain string
	if domain, err := isimApp.Text(ctx, fileEFDomain, "reading EF_DOMAIN"); err == nil {
		homeDomain = domain.String()
	} else {
		u.logger.Debug("reading EF_DOMAIN failed", "err", err)
	}
	impu, err := isimApp.FirstText(ctx, fileEFIMPU, "reading EF_IMPU")
	if err != nil {
		return fmt.Errorf("reading EF_IMPU: %w", err)
	}
	publicIdentity := impu.String()
	serviceCenter := u.serviceCenter
	if psi, err := isimApp.FirstText(ctx, pathEFPSISMSC, "reading EFPSISMSC"); err == nil {
		serviceCenter.PSI = psi.String()
	} else {
		u.logger.Debug("reading EFPSISMSC from ISIM failed", "err", err)
	}
	if homeDomain == "" && strings.Contains(privateIdentity, "@") {
		if _, domain, ok := strings.Cut(privateIdentity, "@"); ok {
			homeDomain = domain
		}
	}
	u.privateIdentity = privateIdentity
	u.publicIdentity = publicIdentity
	u.homeDomain = homeDomain
	u.serviceCenter = serviceCenter
	return nil
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func firstSMSC(records [][]byte) (simfile.SMSC, error) {
	for _, record := range records {
		var smsc simfile.SMSC
		if err := smsc.UnmarshalBinary(record); err != nil {
			return "", err
		}
		if smsc != "" {
			return smsc, nil
		}
	}
	return "", errors.New("reading EF_SMSP: SMSC not found")
}

func formatMNC(imsi string, mncLength int) (string, error) {
	if len(imsi) < 3+mncLength {
		return "", errors.New("IMSI is too short for MNC length")
	}
	switch mncLength {
	case 2:
		return "0" + imsi[3:5], nil
	case 3:
		return imsi[3:6], nil
	default:
		return "", fmt.Errorf("unsupported MNC length %d", mncLength)
	}
}
