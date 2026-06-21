package mbim

type MessageType uint32

const (
	MessageTypeOpen      MessageType = 0x00000001
	MessageTypeClose     MessageType = 0x00000002
	MessageTypeCommand   MessageType = 0x00000003
	MessageTypeHostError MessageType = 0x00000004

	MessageTypeOpenDone       MessageType = 0x80000001
	MessageTypeCloseDone      MessageType = 0x80000002
	MessageTypeCommandDone    MessageType = 0x80000003
	MessageTypeFunctionError  MessageType = 0x80000004
	MessageTypeIndicateStatus MessageType = 0x80000007
)

type SubscriberReadyState uint32

const (
	SubscriberReadyStateNotInitialized SubscriberReadyState = iota
	SubscriberReadyStateInitialized
	SubscriberReadyStateSIMNotInserted
	SubscriberReadyStateBadSIM
	SubscriberReadyStateFailure
	SubscriberReadyStateNotActivated
	SubscriberReadyStateDeviceLocked
	SubscriberReadyStateNoESIMProfile
)

type ReadyInfo uint32

const (
	ReadyInfoNone            ReadyInfo = 0
	ReadyInfoProtectUniqueID ReadyInfo = 1 << 0
)

type CommandType uint32

const (
	CommandTypeQuery CommandType = iota
	CommandTypeSet
)

type UiccApplicationType uint32

const (
	UiccApplicationTypeUnknown UiccApplicationType = iota
	UiccApplicationTypeMF
	UiccApplicationTypeMFSIM
	UiccApplicationTypeMFRUIM
	UiccApplicationTypeUSIM
	UiccApplicationTypeCSIM
	UiccApplicationTypeISIM
)

type UiccSecureMessaging uint32

const (
	UiccSecureMessagingNone UiccSecureMessaging = iota
	UiccSecureMessagingNoHeaderAuth
)

type UiccClassByteType uint32

const (
	UiccClassByteTypeInterIndustry UiccClassByteType = iota
	UiccClassByteTypeExtended
)

type UiccPassThroughAction uint32

const (
	UiccPassThroughActionDisable UiccPassThroughAction = iota
	UiccPassThroughActionEnable
)

type UiccPassThroughStatus uint32

const (
	UiccPassThroughStatusDisabled UiccPassThroughStatus = iota
	UiccPassThroughStatusEnabled
)

type UiccFileAccessibility uint32

const (
	UiccFileAccessibilityUnknown UiccFileAccessibility = iota
	UiccFileAccessibilityNotShareable
	UiccFileAccessibilityShareable
)

type UiccFileType uint32

const (
	UiccFileTypeUnknown UiccFileType = iota
	UiccFileTypeWorkingEF
	UiccFileTypeInternalEF
	UiccFileTypeDFOrADF
)

type UiccFileStructure uint32

const (
	UiccFileStructureUnknown UiccFileStructure = iota
	UiccFileStructureTransparent
	UiccFileStructureCyclic
	UiccFileStructureLinear
	UiccFileStructureBERTLV
)

type PinType uint32

const (
	PinTypeUnknown PinType = iota
	PinTypeCustom
	PinTypePIN1
	PinTypePIN2
	PinTypeDeviceSIM
	PinTypeDeviceFirstSIM
	PinTypeNetwork
	PinTypeNetworkSubset
	PinTypeServiceProvider
	PinTypeCorporate
	PinTypeSubsidy
	PinTypePUK1
	PinTypePUK2
	PinTypeDeviceFirstSIMPUK
	PinTypeNetworkPUK
	PinTypeNetworkSubsetPUK
	PinTypeServiceProviderPUK
	PinTypeCorporatePUK
	PinTypeNEV
	PinTypeADM
)

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

type Application struct {
	AID   []byte
	Label string
}

type FileRef struct {
	AID  []byte
	Path []byte
}

type FileAttributes struct {
	FileStructure FileStructure
	FileType      FileType
	RecordSize    uint16
	RecordCount   uint16
	FileSize      uint16
}

type TransparentRead struct {
	File   FileRef
	Offset uint16
	Length uint16
}

type RecordRead struct {
	File   FileRef
	Record uint16
}
