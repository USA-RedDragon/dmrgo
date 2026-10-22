package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// FeatureSetID is the 8-bit Feature set ID (FID) of a full LC or CSBK. 0 is
// the standardized feature set and 4 to 127 identify a manufacturer. ETSI
// publishes the manufacturer list at
// http://www.etsi.org/images/files/DMRcodes/dmrs-mfid.xls.
// (ETSI TS 102 361-1 clause 9.3.5, Table 9.21)
type FeatureSetID byte

// The FeatureSetID values.
const (
	FIDStandardized        FeatureSetID = 0x0  // standardized feature set
	FIDFlydeMicro          FeatureSetID = 0x4  // Flyde Micro
	FIDProdEl              FeatureSetID = 0x5  // PROD-EL
	FIDTridentMicroSystems FeatureSetID = 0x6  // Trident Micro Systems
	FIDRadiodata           FeatureSetID = 0x7  // RADIODATA
	FIDHYTScienceTech      FeatureSetID = 0x8  // HYT Science Tech
	FIDAselsan             FeatureSetID = 0x9  // ASELSAN
	FIDKirisun             FeatureSetID = 0xA  // Kirisun
	FIDDMRAssociation      FeatureSetID = 0xB  // DMR Association
	FIDMotorola            FeatureSetID = 0x10 // Motorola
	FIDEMC                 FeatureSetID = 0x13 // EMC
	FIDEMC2                FeatureSetID = 0x1C // EMC, second ID
	FIDJVCKenwood          FeatureSetID = 0x20 // JVC Kenwood
	FIDRadioActivity       FeatureSetID = 0x33 // Radio Activity
	FIDRadioActivity2      FeatureSetID = 0x3C // Radio Activity, second ID
	FIDTait                FeatureSetID = 0x58 // Tait
	FIDHYTScienceTech2     FeatureSetID = 0x68 // HYT Science Tech, second ID
	FIDVertexStandard      FeatureSetID = 0x77 // Vertex Standard
)

// String returns the name of the FeatureSetID. It allocates only for a value
// without a name.
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

	return "Unknown FeatureSetID: " + text.Int(int64(fsid))
}
