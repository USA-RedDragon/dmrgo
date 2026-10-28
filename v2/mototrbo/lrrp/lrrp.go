package lrrp

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// DocumentID identifies the kind of a message. Radios send the odd values,
// which carry no constant data table.
type DocumentID uint8

// The DocumentID values.
const (
	DocumentImmediateRequest  DocumentID = 0x05 // request one position
	DocumentImmediateReport   DocumentID = 0x07 // the answer to a DocumentImmediateRequest
	DocumentTriggeredRequest  DocumentID = 0x09 // request positions on a trigger, such as a period
	DocumentTriggeredAnswer   DocumentID = 0x0B // the answer to a DocumentTriggeredRequest
	DocumentTriggeredReport   DocumentID = 0x0D // a position that a trigger sent
	DocumentStopRequest       DocumentID = 0x0F // stop a DocumentTriggeredRequest
	DocumentStopAnswer        DocumentID = 0x11 // the answer to a DocumentStopRequest
	DocumentUnsolicitedReport DocumentID = 0x13 // a position sent without a request
	DocumentProtocolRequest   DocumentID = 0x14 // request the protocol version
	DocumentProtocolReport    DocumentID = 0x15 // the answer to a DocumentProtocolRequest
)

// String returns the name of d. It allocates only for a value without a
// name.
func (d DocumentID) String() string {
	switch d {
	case DocumentImmediateRequest:
		return "Immediate Location Request"
	case DocumentImmediateReport:
		return "Immediate Location Report"
	case DocumentTriggeredRequest:
		return "Triggered Location Request"
	case DocumentTriggeredAnswer:
		return "Triggered Location Answer"
	case DocumentTriggeredReport:
		return "Triggered Location Report"
	case DocumentStopRequest:
		return "Triggered Location Stop Request"
	case DocumentStopAnswer:
		return "Triggered Location Stop Answer"
	case DocumentUnsolicitedReport:
		return "Unsolicited Location Report"
	case DocumentProtocolRequest:
		return "Location Protocol Request"
	case DocumentProtocolReport:
		return "Location Protocol Report"
	}
	return "DocumentID(" + text.Int(int64(d)) + ")"
}

// ResultCode is the result code of an answer or report (tokens 0x37 and
// 0x39). The named values are the codes that open implementations use.
type ResultCode uint32

// The ResultCode values.
const (
	ResultSuccess          ResultCode = 0x00  // the request succeeded
	ResultInvalidCommand   ResultCode = 0x0A  // the radio cannot process the request
	ResultNoGPS            ResultCode = 0x10  // the radio has no GPS fix
	ResultDuplicateRequest ResultCode = 0x16  // the request ID is already in use
	ResultGPSInitializing  ResultCode = 0x200 // the GPS receiver has no good fix yet
)

// String returns the name of r. It allocates only for a value without a
// name.
func (r ResultCode) String() string {
	switch r {
	case ResultSuccess:
		return "Success"
	case ResultInvalidCommand:
		return "Invalid Command"
	case ResultNoGPS:
		return "No GPS"
	case ResultDuplicateRequest:
		return "Duplicate Request"
	case ResultGPSInitializing:
		return "GPS Initializing"
	}
	return "ResultCode(" + text.Int(int64(r)) + ")"
}

// TriggerCondition is the condition of a triggered request that sends a
// report (token 0x4A). The named values are the conditions that captures
// show.
type TriggerCondition uint32

// The TriggerCondition values.
const (
	// TriggerEmergency sends a report when the radio declares an
	// emergency.
	TriggerEmergency TriggerCondition = 2
)

// String returns the name of c. It allocates only for a value without a
// name.
func (c TriggerCondition) String() string {
	if c == TriggerEmergency {
		return "Emergency"
	}
	return "TriggerCondition(" + text.Int(int64(c)) + ")"
}

// Time is the UTC time of a position (token 0x34).
//
//dmr:frame 5
type Time struct {
	// Year is the year, such as 2026.
	Year uint16 `dmr:"bits:0-13"`
	// Month is the month, 1 to 12.
	Month uint8 `dmr:"bits:14-17"`
	// Day is the day of the month, 1 to 31.
	Day uint8 `dmr:"bits:18-22"`
	// Hour is the hour, 0 to 23.
	Hour uint8 `dmr:"bits:23-27"`
	// Minute is the minute, 0 to 59.
	Minute uint8 `dmr:"bits:28-33"`
	// Second is the second, 0 to 59.
	Second uint8 `dmr:"bits:34-39"`
}

// Point2D is a position (token 0x66).
//
//dmr:stream
type Point2D struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
}

// Circle2D is a position with its accuracy (token 0x51).
//
//dmr:stream
type Circle2D struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Radius is the radius of the circle that holds the position, in
	// meters.
	Radius float64 `dmr:"ufloatvar"`
}

// Point3D is a position with an altitude (token 0x69).
//
//dmr:stream
type Point3D struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Altitude is in meters.
	Altitude float64 `dmr:"sfloatvar"`
}

// Point3DWhole is a position with an altitude in whole meters (token 0x67),
// which radios in Buenos Aires send in place of Point3D.
//
//dmr:stream
type Point3DWhole struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Altitude is in meters.
	Altitude int32 `dmr:"sintvar"`
}

// Circle3DWhole is a position with an altitude and the accuracy of both,
// in whole meters (token 0x53), which radios in Buenos Aires send in place
// of Circle3D.
//
//dmr:stream
type Circle3DWhole struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Radius is the radius of the circle that holds the position, in
	// meters.
	Radius uint32 `dmr:"uintvar"`
	// Altitude is in meters.
	Altitude int32 `dmr:"sintvar"`
	// AltitudeAccuracy is in meters.
	AltitudeAccuracy uint32 `dmr:"uintvar"`
}

// Point3DAccuracy is a position with an altitude and its accuracy (token
// 0x6A).
//
//dmr:stream
type Point3DAccuracy struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Altitude is in meters.
	Altitude float64 `dmr:"sfloatvar"`
	// AltitudeAccuracy is in meters.
	AltitudeAccuracy float64 `dmr:"ufloatvar"`
}

// Circle3D is a position with an altitude and the accuracy of both (token
// 0x55).
//
//dmr:stream
type Circle3D struct {
	// Latitude is in degrees, north positive. Its top bit is the sign
	// and the other 31 bits the magnitude, as radios send it.
	Latitude float64 `dmr:"i32,signmag,scale:90/31"`
	// Longitude is in degrees, east positive.
	Longitude float64 `dmr:"i32,wrap,scale:360/32"`
	// Radius is the radius of the circle that holds the position, in
	// meters.
	Radius float64 `dmr:"ufloatvar"`
	// Altitude is in meters.
	Altitude float64 `dmr:"sfloatvar"`
	// AltitudeAccuracy is in meters.
	AltitudeAccuracy float64 `dmr:"ufloatvar"`
}

// OperationError is the result of a request that failed, with its text
// (token 0x39).
//
//dmr:stream
type OperationError struct {
	// Code is the result code.
	Code ResultCode `dmr:"uintvar"`
	// Info is the text of the error.
	Info []byte `dmr:"lvvar"`
}

// Report holds the tokens of a report or an answer. Each value field has a
// Has field that shows whether the message carries it.
//
//dmr:tokens
type Report struct {
	// RequestID is the identifier of the request that the report answers.
	RequestID    []byte `dmr:"lvvar,token:0x22"`
	HasRequestID bool   `dmr:"-"`
	// ShortRequestID is a request identifier of one octet.
	ShortRequestID    uint8 `dmr:"u8,token:0x23"`
	HasShortRequestID bool  `dmr:"-"`
	// Time is the time of the position.
	Time    Time `dmr:"frame,token:0x34"`
	HasTime bool `dmr:"-"`
	// ProtocolVersion is the version of the protocol.
	ProtocolVersion    uint32 `dmr:"uintvar,token:0x36"`
	HasProtocolVersion bool   `dmr:"-"`
	// Result is the result code of the request.
	Result    ResultCode `dmr:"uintvar,token:0x37"`
	HasResult bool       `dmr:"-"`
	// Success reports a request that succeeded.
	Success bool `dmr:"flag,token:0x38"`
	// Error is the result of a request that failed.
	Error    OperationError `dmr:"stream,token:0x39"`
	HasError bool           `dmr:"-"`
	// Circle2D is a position with its accuracy.
	Circle2D    Circle2D `dmr:"stream,token:0x51"`
	HasCircle2D bool     `dmr:"-"`
	// Circle3DWhole is a position with an altitude and its accuracy in
	// whole meters.
	Circle3DWhole    Circle3DWhole `dmr:"stream,token:0x53"`
	HasCircle3DWhole bool          `dmr:"-"`
	// Circle3D is a position with an altitude and its accuracy.
	Circle3D    Circle3D `dmr:"stream,token:0x55"`
	HasCircle3D bool     `dmr:"-"`
	// Direction is the heading in units of 2 degrees from north.
	Direction    uint8 `dmr:"u8,token:0x56"`
	HasDirection bool  `dmr:"-"`
	// LevelOfConfidence is the confidence of the position in percent.
	LevelOfConfidence    uint8 `dmr:"u8,token:0x65"`
	HasLevelOfConfidence bool  `dmr:"-"`
	// Point2D is a position.
	Point2D    Point2D `dmr:"stream,token:0x66"`
	HasPoint2D bool    `dmr:"-"`
	// Point3DWhole is a position with an altitude in whole meters.
	Point3DWhole    Point3DWhole `dmr:"stream,token:0x67"`
	HasPoint3DWhole bool         `dmr:"-"`
	// Point3D is a position with an altitude.
	Point3D    Point3D `dmr:"stream,token:0x69"`
	HasPoint3D bool    `dmr:"-"`
	// Point3DAccuracy is a position with an altitude and its accuracy.
	Point3DAccuracy    Point3DAccuracy `dmr:"stream,token:0x6A"`
	HasPoint3DAccuracy bool            `dmr:"-"`
	// SpeedWhole is the horizontal speed in whole meters per second, which
	// some radios send in place of Speed.
	SpeedWhole    uint32 `dmr:"uintvar,token:0x6B"`
	HasSpeedWhole bool   `dmr:"-"`
	// Speed is the horizontal speed in meters per second.
	Speed    float64 `dmr:"ufloatvar,token:0x6C"`
	HasSpeed bool    `dmr:"-"`
	// VerticalSpeed is the vertical speed in meters per second, up
	// positive.
	VerticalSpeed    float64 `dmr:"sfloatvar,token:0x70"`
	HasVerticalSpeed bool    `dmr:"-"`
}

// Request holds the tokens of a request. AppendBinary writes them in the
// order of the fields, which is the order that radios and servers use.
//
//dmr:tokens
type Request struct {
	// RequestID is the identifier that the reports carry.
	RequestID    []byte `dmr:"lvvar,token:0x22"`
	HasRequestID bool   `dmr:"-"`
	// ShortRequestID is a request identifier of one octet.
	ShortRequestID    uint8 `dmr:"u8,token:0x23"`
	HasShortRequestID bool  `dmr:"-"`
	// Periodic requests reports on a period or a distance.
	Periodic bool `dmr:"flag,token:0x34"`
	// OneShot requests a single report.
	OneShot bool `dmr:"flag,token:0x33"`
	// Interval is the period of a periodic trigger, in seconds.
	Interval    uint32 `dmr:"uintvar,token:0x31"`
	HasInterval bool   `dmr:"-"`
	// Distance is the distance of a distance trigger, in meters.
	Distance    uint32 `dmr:"uintvar,token:0x78"`
	HasDistance bool   `dmr:"-"`
	// TriggerCondition is the condition that sends a report.
	TriggerCondition    TriggerCondition `dmr:"uintvar,token:0x4A"`
	HasTriggerCondition bool             `dmr:"-"`
	// ProtocolVersion is the requested protocol version.
	ProtocolVersion    uint32 `dmr:"uintvar,token:0x3F"`
	HasProtocolVersion bool   `dmr:"-"`
	// Accuracy requests the accuracy of the position.
	Accuracy bool `dmr:"flag,token:0x50"`
	// AccuracyAndTime requests the accuracy and the time of the position.
	AccuracyAndTime bool `dmr:"flag,token:0x51"`
	// MaxInfoAge is the age, in seconds, of the oldest position that the
	// report may carry: the require-max-info-age token.
	MaxInfoAge    uint32 `dmr:"uintvar,token:0x42"`
	HasMaxInfoAge bool   `dmr:"-"`
	// Time requests the time of the position.
	Time bool `dmr:"flag,token:0x52"`
	// Altitude requests the altitude.
	Altitude bool `dmr:"flag,token:0x54"`
	// Speed requests the horizontal speed.
	Speed bool `dmr:"flag,token:0x62"`
	// Direction requests the heading.
	Direction bool `dmr:"flag,token:0x57"`
	// VerticalSpeed requests the vertical speed.
	VerticalSpeed bool `dmr:"flag,token:0x64"`
	// LevelOfConfidence is the confidence that the request specifies.
	LevelOfConfidence    uint8 `dmr:"u8,token:0x61"`
	HasLevelOfConfidence bool  `dmr:"-"`
}

// Message is a MOTOTRBO Location Request and Response Protocol message, the
// payload of a UDP datagram to port 4001. It is larger than 256 octets, so
// decode it with DecodeMessageInto into a Message that lives in a struct of
// the caller or a package variable.
//
//dmr:stream into
type Message struct {
	// Document is the kind of the message.
	Document DocumentID `dmr:"u8"`
	// Length is the number of octets after it. AppendBinary sets it.
	Length uint32 `dmr:"uintvar,length"`
	// Request holds the tokens of a request document.
	Request Request `dmr:"tokens,if:Document==DocumentImmediateRequest|DocumentTriggeredRequest|DocumentStopRequest|DocumentProtocolRequest"`
	// Report holds the tokens of a report or answer document.
	Report Report `dmr:"tokens,if:Document!=DocumentImmediateRequest|DocumentTriggeredRequest|DocumentStopRequest|DocumentProtocolRequest"`
}
