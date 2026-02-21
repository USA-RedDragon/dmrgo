package enums

import "fmt"

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.5  Feature set ID (FID)
//
// list of specific manufacturers: http://www.etsi.org/images/files/DMRcodes/dmrs-mfid.xls
type FeatureSetID byte

const (
	StandardizedFID             FeatureSetID = 0x0
	FlydeMicroLtd               FeatureSetID = 0x4
	ProdElSpa                   FeatureSetID = 0x5
	TridentMicroSystems         FeatureSetID = 0x6
	RadiodataGmbh               FeatureSetID = 0x7
	HytScienceTech              FeatureSetID = 0x8
	AselsanElektronik           FeatureSetID = 0x9
	KirisunCommunications       FeatureSetID = 0xA
	DmrAssociationLtd           FeatureSetID = 0xB
	MotorolaLtd                 FeatureSetID = 0x10
	ElectronicMarketingCompany  FeatureSetID = 0x13
	ElectronicMarketingCompany2 FeatureSetID = 0x1C
	JvcKenwood                  FeatureSetID = 0x20
	RadioActivity               FeatureSetID = 0x33
	RadioActivity2              FeatureSetID = 0x3C
	TaitElectronicsLtd          FeatureSetID = 0x58
	HytScienceTech2             FeatureSetID = 0x68
	VertexStandard              FeatureSetID = 0x77
)

func FeatureSetIDToName(fsid FeatureSetID) string {
	switch fsid {
	case StandardizedFID:
		return "Standardized FID"
	case FlydeMicroLtd:
		return "Flyde Micro Ltd. UK"
	case ProdElSpa:
		return "PROD-EL Spa. Italy"
	case TridentMicroSystems:
		return "Trident Datacom DBA Trident Micro Systems USA"
	case RadiodataGmbh:
		return "RADIODATA GmbH Germany"
	case HytScienceTech:
		return "HYT Science Tech China"
	case AselsanElektronik:
		return "ASELSAN Elektronik Sanayi ve Ticaret A.S. Turket"
	case KirisunCommunications:
		return "Kirisun Communications Co. Ltd. China"
	case DmrAssociationLtd:
		return "DMR Association Ltd. UK"
	case MotorolaLtd:
		return "Motorola Ltd. UK"
	case ElectronicMarketingCompany:
		return "EMC S.p.A. (Electronic Marketing Company) Italy"
	case ElectronicMarketingCompany2:
		return "EMC S.p.A. (Electronic Marketing Company) Italy"
	case JvcKenwood:
		return "JVC Kenwood Corporation Japan"
	case RadioActivity:
		return "Radio Activity Srl. Italy"
	case RadioActivity2:
		return "Radio Activity Srl. Italy"
	case TaitElectronicsLtd:
		return "Tait Electronics Ltd. New Zealand"
	case HytScienceTech2:
		return "Hyt Science & Tech China"
	case VertexStandard:
		return "Vertex Standard UK"
	}

	return fmt.Sprintf("Unknown FeatureSetID: %d", fsid)
}

func FeatureSetIDFromInt(i int) (FeatureSetID, error) {
	switch FeatureSetID(i) {
	case StandardizedFID:
		return StandardizedFID, nil
	case FlydeMicroLtd:
		return FlydeMicroLtd, nil
	case ProdElSpa:
		return ProdElSpa, nil
	case TridentMicroSystems:
		return TridentMicroSystems, nil
	case RadiodataGmbh:
		return RadiodataGmbh, nil
	case HytScienceTech:
		return HytScienceTech, nil
	case AselsanElektronik:
		return AselsanElektronik, nil
	case KirisunCommunications:
		return KirisunCommunications, nil
	case DmrAssociationLtd:
		return DmrAssociationLtd, nil
	case MotorolaLtd:
		return MotorolaLtd, nil
	case ElectronicMarketingCompany:
		return ElectronicMarketingCompany, nil
	case ElectronicMarketingCompany2:
		return ElectronicMarketingCompany2, nil
	case JvcKenwood:
		return JvcKenwood, nil
	case RadioActivity:
		return RadioActivity, nil
	case RadioActivity2:
		return RadioActivity2, nil
	case TaitElectronicsLtd:
		return TaitElectronicsLtd, nil
	case HytScienceTech2:
		return HytScienceTech2, nil
	case VertexStandard:
		return VertexStandard, nil
	}
	return StandardizedFID, fmt.Errorf("unknown feature set id: %d", i)
}
