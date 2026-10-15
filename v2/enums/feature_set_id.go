package enums

import "fmt"

// ETSI TS 102 361-1 - 9.3.5  Feature set ID (FID)
//
// list of specific manufacturers: http://www.etsi.org/images/files/DMRcodes/dmrs-mfid.xls
type FeatureSetID byte

const (
	FIDStandardized        FeatureSetID = 0x0
	FIDFlydeMicro          FeatureSetID = 0x4
	FIDProdEl              FeatureSetID = 0x5
	FIDTridentMicroSystems FeatureSetID = 0x6
	FIDRadiodata           FeatureSetID = 0x7
	FIDHYTScienceTech      FeatureSetID = 0x8
	FIDAselsan             FeatureSetID = 0x9
	FIDKirisun             FeatureSetID = 0xA
	FIDDMRAssociation      FeatureSetID = 0xB
	FIDMotorola            FeatureSetID = 0x10
	FIDEMC                 FeatureSetID = 0x13
	FIDEMC2                FeatureSetID = 0x1C
	FIDJVCKenwood          FeatureSetID = 0x20
	FIDRadioActivity       FeatureSetID = 0x33
	FIDRadioActivity2      FeatureSetID = 0x3C
	FIDTait                FeatureSetID = 0x58
	FIDHYTScienceTech2     FeatureSetID = 0x68
	FIDVertexStandard      FeatureSetID = 0x77
)

func (fsid FeatureSetID) String() string {
	switch fsid {
	case FIDStandardized:
		return "Standardized FID"
	case FIDFlydeMicro:
		return "Flyde Micro Ltd. UK"
	case FIDProdEl:
		return "PROD-EL Spa. Italy"
	case FIDTridentMicroSystems:
		return "Trident Datacom DBA Trident Micro Systems USA"
	case FIDRadiodata:
		return "RADIODATA GmbH Germany"
	case FIDHYTScienceTech:
		return "HYT Science Tech China"
	case FIDAselsan:
		return "ASELSAN Elektronik Sanayi ve Ticaret A.S. Turkey"
	case FIDKirisun:
		return "Kirisun Communications Co. Ltd. China"
	case FIDDMRAssociation:
		return "DMR Association Ltd. UK"
	case FIDMotorola:
		return "Motorola Ltd. UK"
	case FIDEMC:
		return "EMC S.p.A. (Electronic Marketing Company) Italy"
	case FIDEMC2:
		return "EMC S.p.A. (Electronic Marketing Company) Italy"
	case FIDJVCKenwood:
		return "JVC Kenwood Corporation Japan"
	case FIDRadioActivity:
		return "Radio Activity Srl. Italy"
	case FIDRadioActivity2:
		return "Radio Activity Srl. Italy"
	case FIDTait:
		return "Tait Electronics Ltd. New Zealand"
	case FIDHYTScienceTech2:
		return "HYT Science Tech China"
	case FIDVertexStandard:
		return "Vertex Standard UK"
	}

	return fmt.Sprintf("Unknown FeatureSetID: %d", fsid)
}

// Valid reports whether f is a value that ETSI TS 102 361 defines.
func (f FeatureSetID) Valid() bool {
	switch f {
	case FIDStandardized,
		FIDFlydeMicro,
		FIDProdEl,
		FIDTridentMicroSystems,
		FIDRadiodata,
		FIDHYTScienceTech,
		FIDAselsan,
		FIDKirisun,
		FIDDMRAssociation,
		FIDMotorola,
		FIDEMC,
		FIDEMC2,
		FIDJVCKenwood,
		FIDRadioActivity,
		FIDRadioActivity2,
		FIDTait,
		FIDHYTScienceTech2,
		FIDVertexStandard:
		return true
	}
	return false
}
