package trunk

import "github.com/USA-RedDragon/dmrgo/v2/layer3"

// NMEAShort is the Short NMEA (IEC 61162-1) appended data of a UDT: one
// block with the position, speed and the time in steps of 10 s. Decode it
// from the Data of an Appended with 1 block. (ETSI TS 102 361-4 clause
// B.3.6.1, Table B.7)
type NMEAShort struct {
	Encrypted   bool   `dmr:"bit:0"`      // C: the data is encrypted
	North       bool   `dmr:"bit:1"`      // NS: north latitude, false for south
	East        bool   `dmr:"bit:2"`      // EW: east longitude, false for west
	Fix         bool   `dmr:"bit:3"`      // Q: the GPS fix is valid
	Speed       uint8  `dmr:"bits:4-10"`  // SPEED: knots, 0 to 126, 127 for more than 126
	LatDegrees  uint8  `dmr:"bits:11-17"` // NDEG: latitude degrees, 0 to 89
	LatMinutes  uint8  `dmr:"bits:18-23"` // NMINmm: latitude minutes, 0 to 59
	LatFraction uint16 `dmr:"bits:24-37"` // NMINF: latitude minutes in 1/10000, 0 to 9999
	LonDegrees  uint8  `dmr:"bits:38-45"` // EDEG: longitude degrees, 0 to 179
	LonMinutes  uint8  `dmr:"bits:46-51"` // EMINmm: longitude minutes, 0 to 59
	LonFraction uint16 `dmr:"bits:52-65"` // EMINF: longitude minutes in 1/10000, 0 to 9999
	Hours       uint8  `dmr:"bits:66-70"` // UTChh: UTC hours, 0 to 23
	Minutes     uint8  `dmr:"bits:71-76"` // UTCmm: UTC minutes, 0 to 59
	TenSeconds  uint8  `dmr:"bits:77-79"` // UTCss3: UTC seconds in steps of 10, 0 to 5
}

// NMEALong is the Long NMEA (IEC 61162-1) Format Specified appended data
// of a UDT: two blocks with the fields of NMEAShort, the time in seconds
// and the course over ground. Decode it from the Data of an Appended with 2
// blocks. (ETSI TS 102 361-4 clause B.3.6.2, Table B.7)
type NMEALong struct {
	Encrypted   bool   `dmr:"bit:0"`        // C: the data is encrypted
	North       bool   `dmr:"bit:1"`        // NS: north latitude, false for south
	East        bool   `dmr:"bit:2"`        // EW: east longitude, false for west
	Fix         bool   `dmr:"bit:3"`        // Q: the GPS fix is valid
	Speed       uint8  `dmr:"bits:4-10"`    // SPEED: knots, 0 to 126, 127 for more than 126
	LatDegrees  uint8  `dmr:"bits:11-17"`   // NDEG: latitude degrees, 0 to 89
	LatMinutes  uint8  `dmr:"bits:18-23"`   // NMINmm: latitude minutes, 0 to 59
	LatFraction uint16 `dmr:"bits:24-37"`   // NMINF: latitude minutes in 1/10000, 0 to 9999
	LonDegrees  uint8  `dmr:"bits:38-45"`   // EDEG: longitude degrees, 0 to 179
	LonMinutes  uint8  `dmr:"bits:46-51"`   // EMINmm: longitude minutes, 0 to 59
	LonFraction uint16 `dmr:"bits:52-65"`   // EMINF: longitude minutes in 1/10000, 0 to 9999
	Hours       uint8  `dmr:"bits:66-70"`   // UTChh: UTC hours, 0 to 23
	Minutes     uint8  `dmr:"bits:71-76"`   // UTCmm: UTC minutes, 0 to 59
	Seconds     uint8  `dmr:"bits:77-82"`   // UTCss6: UTC seconds, 0 to 59
	Spare1      uint8  `dmr:"bits:83-87"`   // spare, free for manufacturers
	MFID        uint8  `dmr:"bits:88-95"`   // MFID: the feature set ID, 0 for SFID
	Spare2      uint8  `dmr:"bits:96-102"`  // spare, free for manufacturers
	Course      uint16 `dmr:"bits:103-111"` // COG: course over ground in degrees, 0 to 359
	Spare3      uint64 `dmr:"bits:112-175"` // spare, free for manufacturers
}

// LIPShortReport is the LIP Short Location Report that a UDT in the LIP
// format carries: PDU type 0, the position, its error, the speed and the
// direction. Decode it from the first 10 octets of the LIP data. (ETSI TS
// 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 clause 6.2.1)
type LIPShortReport struct {
	PDUType        uint8                `dmr:"bits:0-1"`                  // PDU type, 0 for a short location report
	TimeElapsed    uint8                `dmr:"bits:2-3"`                  // time since the position was found: 0 under 5 s, 1 under 5 min, 2 under 30 min, 3 unknown
	Longitude      float64              `dmr:"bits:4-28,type:longitude"`  // degrees, -180 to 180
	Latitude       float64              `dmr:"bits:29-52,type:latitude"`  // degrees, -90 to 90
	PositionError  layer3.PositionError `dmr:"bits:53-55,delegate,noptr"` // position error
	Velocity       uint8                `dmr:"bits:56-62"`                // horizontal velocity code, 127 for unknown
	Direction      uint8                `dmr:"bits:63-66"`                // direction of travel in steps of 22.5 degrees, 0 is north
	AdditionalType bool                 `dmr:"bit:67"`                    // AdditionalData is user defined, false for the reason for sending
	AdditionalData uint8                `dmr:"bits:68-75"`                // reason for sending or user defined data
}

// USBDPollRequest is the Poll Request of the USBD polling service: the
// payload of an outbound USBD after its Service Type. Decode it from the
// Payload of a pdu.UnifiedSingleBlockData. (ETSI TS 102 361-4 clause
// 6.6.11.1, Table 6.76)
type USBDPollRequest struct {
	ResponseDelay   uint8  `dmr:"bits:0-1"`   // RD: the poll response is due 1 to 4 TDMA frames later
	PayloadContents bool   `dmr:"bit:2"`      // PC: Parameters holds data
	Reserved        bool   `dmr:"bit:3"`      // reserved, false
	Parameters      uint64 `dmr:"bits:4-51"`  // 48 bits that depend on the Service Type
	Target          uint32 `dmr:"bits:52-75"` // MS target address
}

// USBDLIPResponse is the Poll Response of the USBD polling service for
// LIP: the payload of an inbound USBD with Service Type LIP. Encode it into
// the Payload of a pdu.UnifiedSingleBlockData. (ETSI TS 102 361-4 clause
// 6.6.11.3.2, Table 6.79)
type USBDLIPResponse struct {
	TimeElapsed   uint8                `dmr:"bits:0-1"`                  // time since the position was found, as in LIPShortReport
	Longitude     float64              `dmr:"bits:2-26,type:longitude"`  // degrees, -180 to 180
	Latitude      float64              `dmr:"bits:27-50,type:latitude"`  // degrees, -90 to 90
	PositionError layer3.PositionError `dmr:"bits:51-53,delegate,noptr"` // position error
	Velocity      uint8                `dmr:"bits:54-60"`                // horizontal velocity code, 127 for unknown
	Direction     uint8                `dmr:"bits:61-64"`                // direction of travel in steps of 22.5 degrees
	Reason        uint8                `dmr:"bits:65-67"`                // reason for sending, 0 for a response to an immediate request
	HashedSource  uint8                `dmr:"bits:68-75"`                // 8-bit hash of the source address, as pdu.AddressHash
}
