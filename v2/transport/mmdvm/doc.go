// Package mmdvm encodes and decodes the serial protocol between a host such
// as MMDVMHost and an MMDVM or MMDVM_HS modem, for both roles. It does no
// IO and keeps no state.
//
// Next finds frames in the octets read from the port, and AppendFrame
// builds one around a payload. The typed messages decode the payload of a
// frame of their command and append it back: DMRData and Debug (which take
// the whole Frame, since the command gives the slot or the count of
// numbers), Version, StatusV1 and StatusV2, ConfigV1 and ConfigV2,
// SetFreq, SetMode, Ack, Nak, DMRStart, DMRAbort, DMRShortLC and DMRAloha.
// Protocol 1 is the MMDVM_HS firmware and protocol 2 the MMDVM firmware;
// the version reply tells which. The frames of the other modes stay a
// Frame.
//
// Decode functions never allocate and never panic. They return the static
// errors ErrShort, ErrLength and ErrCommand, which errors.Is matches, and a
// static *dmrgo.FieldRangeError for a field value that the firmware NAKs,
// such as a color code above 15.
//
// (MMDVMHost 9384d68 Modem.cpp; MMDVM 046e1df SerialPort.cpp, DMRSlotRX.cpp,
// DMRIdleRX.cpp and DMRTX.cpp; MMDVM_HS bf66faf SerialPort.cpp)
package mmdvm
