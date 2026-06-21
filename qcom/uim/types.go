package uim

type FileStructure byte

const (
	FileStructureTransparent FileStructure = 0x41
	FileStructureLinearFixed FileStructure = 0x42
)

type FileType byte

const (
	FileTypeWorkingEF FileType = 0x21
	FileTypeDFOrADF   FileType = 0x38
)

type CardState byte

const (
	CardStateAbsent CardState = iota
	CardStatePresent
	CardStateError
)

type PhysicalCardState uint32

const (
	PhysicalCardStateUnknown PhysicalCardState = iota
	PhysicalCardStateAbsent
	PhysicalCardStatePresent
)

type SlotState uint32

const (
	SlotStateInactive SlotState = iota
	SlotStateActive
)

type CardProtocol uint32

const (
	CardProtocolUnknown CardProtocol = iota
	CardProtocolICC
	CardProtocolUICC
)

type QMIFileType byte

const (
	QMIFileTypeTransparent QMIFileType = iota
	QMIFileTypeCyclic
	QMIFileTypeLinearFixed
	QMIFileTypeDedicated
	QMIFileTypeMaster
)

type PINState byte

const (
	PINStateNotInitialized PINState = iota
	PINStateEnabledNotVerified
	PINStateEnabledVerified
	PINStateDisabled
	PINStateBlocked
	PINStatePermanentlyBlocked
)

type CardError byte

const (
	CardErrorUnknown CardError = iota
	CardErrorPowerDown
	CardErrorPoll
	CardErrorNoATRReceived
	CardErrorVoltageMismatch
	CardErrorParity
	CardErrorPossiblyRemoved
	CardErrorTechnical
)

type ApplicationType byte

const (
	ApplicationTypeUnknown ApplicationType = iota
	ApplicationTypeSIM
	ApplicationTypeUSIM
	ApplicationTypeRUIM
	ApplicationTypeCSIM
	ApplicationTypeISIM
)

type ApplicationState byte

const (
	ApplicationStateUnknown ApplicationState = iota
	ApplicationStateDetected
	ApplicationStatePIN1OrUPINRequired
	ApplicationStatePUK1OrUPINRequired
	ApplicationStateCheckPersonalization
	ApplicationStatePIN1Blocked
	ApplicationStateIllegal
	ApplicationStateReady
)

type PersonalizationState byte

const (
	PersonalizationStateUnknown PersonalizationState = iota
	PersonalizationStateInProgress
	PersonalizationStateReady
	PersonalizationStateCodeRequired
	PersonalizationStatePUKCodeRequired
	PersonalizationStatePermanentlyBlocked
)

type PersonalizationFeature byte

const (
	PersonalizationFeatureGWNetwork PersonalizationFeature = iota
	PersonalizationFeatureGWNetworkSubset
	PersonalizationFeatureGWServiceProvider
	PersonalizationFeatureGWCorporate
	PersonalizationFeatureGWUIM
	PersonalizationFeatureOneXNetworkType1
	PersonalizationFeatureOneXNetworkType2
	PersonalizationFeatureOneXHRPD
	PersonalizationFeatureOneXServiceProvider
	PersonalizationFeatureOneXCorporate
	PersonalizationFeatureOneXRUIM
	PersonalizationFeatureGWServiceProviderName
	PersonalizationFeatureGWSPAndEHPLMN
	PersonalizationFeatureGWICCID
	PersonalizationFeatureGWIMPI
	PersonalizationFeatureGWNetworkSubsetServiceProvider
	PersonalizationFeatureGWCarrier
)

type Session uint8

const (
	SessionPrimaryGWProvisioning Session = 0

	SessionNonProvisioningSlot1 Session = 4
	SessionNonProvisioningSlot2 Session = 5
	SessionCardSlot1            Session = 6
	SessionCardSlot2            Session = 7

	SessionNonProvisioningSlot3 Session = 16
	SessionNonProvisioningSlot4 Session = 17
	SessionNonProvisioningSlot5 Session = 18
	SessionCardSlot3            Session = 19
	SessionCardSlot4            Session = 20
	SessionCardSlot5            Session = 21
)

type FileAttributes struct {
	FileStructure FileStructure
	FileType      FileType
	RecordSize    uint16
	RecordCount   uint16
	FileSize      uint16
}

type File struct {
	Session Session
	AID     []byte
	Path    []byte
}

type TransparentRead struct {
	File   File
	Offset uint16
	Length uint16
}

type RecordRead struct {
	File   File
	Record uint16
	Length uint16
}

type AuthContext byte

const (
	AuthContext3G     AuthContext = 3
	AuthContextIMSAKA AuthContext = 11
)

type AuthenticateRequest struct {
	Session Session
	AID     []byte
	Context AuthContext
	Rand    []byte
	AUTN    []byte
}

type EnvelopeResponse struct {
	SW1  byte
	SW2  byte
	Data []byte
}

type PowerOnSIMRequest struct {
	Slot                uint8
	IgnoreHotSwapSwitch bool
}

type ChangeProvisioningSessionRequest struct {
	Session  Session
	Activate bool
	Slot     uint8
	AID      []byte
}

type OpenLogicalChannelRequest struct {
	AID []byte
}

type OpenLogicalChannelResponse struct {
	Channel uint8
}

type CloseLogicalChannelRequest struct {
	Channel uint8
}

type CloseLogicalChannelResponse struct{}

type SendAPDURequest struct {
	Command []byte
}

type SendAPDUResponse struct {
	Response []byte
}

type RefreshStage uint8

const (
	RefreshStageWaitForOK RefreshStage = iota
	RefreshStageStart
	RefreshStageEndWithSuccess
	RefreshStageEndWithFailure
)

type RefreshMode uint8

const (
	RefreshModeReset RefreshMode = iota
	RefreshModeInit
	RefreshModeInitFCN
	RefreshModeFCN
	RefreshModeInitFullFCN
	RefreshModeApplicationReset
	RefreshMode3GReset
)

type RefreshFile struct {
	FileID uint16
	Path   []byte
}

type RefreshEvent struct {
	Stage   RefreshStage
	Mode    RefreshMode
	Session Session
	AID     []byte
	Files   []RefreshFile
}

type RefreshRegisterRequest struct {
	Session     Session
	AID         []byte
	VoteForInit bool
	Files       []RefreshFile
}
