package combustion

import (
	"encoding/hex"
	"slices"

	"tinygo.org/x/bluetooth"
)

const CombustionManufacuterID = 0x09C7

// See https://github.com/combustion-inc/combustion-documentation/blob/main/meatnet_node_ble_specification.rst#product-type
type CombustionProductType byte

// 0: Unknown
// 1: Predictive Probe
// 2: MeatNet Repeater Node (used in Advertisements to show repeated data)
// 3: Giant Grill Gauge
// 4: Display (Timer)
// 5: Booster (Charger)
const (
	CombustionUnknownPT         CombustionProductType = 0
	CombustionPredictiveProbePT CombustionProductType = 1
)

// See https://github.com/combustion-inc/combustion-documentation/blob/main/probe_ble_specification.rst#mode-and-id-data
type CombustionMode byte

const (
	CombustionModeNormal      CombustionMode = 0
	CombustionModeInstantRead CombustionMode = 1
	CombustionModeReserved    CombustionMode = 2
	CombustionModeError       CombustionMode = 3
)

type CombustionColorID byte

const (
	CombustionColorYellow CombustionColorID = 0
	CombustionColorGrey   CombustionColorID = 1
)

type CombustionPacket struct {
	ProductType         CombustionProductType
	SerialNumber        string
	Temps               []float32
	Mode                CombustionMode
	ColorID             CombustionColorID
	ProbeID             byte
	BatteryOK           bool
	VirtualCoreIndex    byte
	VirtualSurfaceIndex byte
	VirtualAmbientIndex byte
	Overheating         [8]bool
}

type Combustion struct {
	BluetoothAdapter *bluetooth.Adapter
	packetHandler    func(*Combustion, CombustionPacket)
}

// Temperature = (raw value * 0.05) - 20
func (c *Combustion) fromRawTemp(raw uint16) float32 {
	return (float32(raw) * 0.05) - 20.0
}

// See https://github.com/combustion-inc/combustion-documentation/blob/main/probe_ble_specification.rst#manufacturer-specific-data
// Data Value           Bytes  	Description
// Product Type			1	   	See Product Type.
// Serial Number		4      	Device serial number
// Raw Temperature Data	13		See Raw Temperature Data.
// Mode/ID				1		See Mode and ID Data.
// Battery Status 		1		See Battery Status and Virtual Sensors.
// Network Information	1		Unused by Probe, D/C
// Overheating Sensors	1		See Overheating Sensors.
func (c *Combustion) ExtractCombustionPacket(rawPacket []byte) *CombustionPacket {
	packet := CombustionPacket{
		ProductType:  CombustionUnknownPT,
		SerialNumber: "",
		BatteryOK:    false,
	}

	packet.ProductType = CombustionProductType(rawPacket[0])

	slices.Reverse(rawPacket[1:5])
	packet.SerialNumber = hex.EncodeToString(rawPacket[1:5])

	packet.Mode = CombustionMode(rawPacket[18] & 0x03)              // bits 1-2 Mode
	packet.ColorID = CombustionColorID((rawPacket[18] >> 2) & 0x70) // bits 3-5 Color ID
	packet.ProbeID = (rawPacket[18] >> 5) & 0x70                    // bits 6-8 Probe ID

	// 1-13		Thermistor 1 raw reading
	// ...
	// 92-104	Thermistor 8 raw reading
	slices.Reverse(rawPacket[5:18])
	if packet.Mode == CombustionModeInstantRead { // Only the first value is populated in this mode
		packet.Temps = make([]float32, 1)
		packet.Temps[0] = c.fromRawTemp((uint16(rawPacket[16]&0x1f) << 8) | uint16(rawPacket[17]&0xff))
	} else if packet.Mode == CombustionModeNormal {
		packet.Temps = make([]float32, 8)
		packet.Temps[7] = c.fromRawTemp((uint16(rawPacket[5]&0xff) << 5) | (uint16(rawPacket[6]) >> 3))
		packet.Temps[6] = c.fromRawTemp((uint16(rawPacket[6]&0x07) << 10) | (uint16(rawPacket[7]&0xff) << 2) | uint16(rawPacket[8]&0xc0)>>6)
		packet.Temps[5] = c.fromRawTemp((uint16(rawPacket[8]&0x3f) << 7) | (uint16(rawPacket[9]&0xfe) >> 1))
		packet.Temps[4] = c.fromRawTemp((uint16(rawPacket[9]&0x01) << 12) | (uint16(rawPacket[10]&0xff) << 4) | uint16(rawPacket[11]&0xf0)>>4)
		packet.Temps[3] = c.fromRawTemp((uint16(rawPacket[11]&0x0f) << 9) | (uint16(rawPacket[12]&0xff) << 1) | uint16(rawPacket[13]&0x80)>>7)
		packet.Temps[2] = c.fromRawTemp((uint16(rawPacket[13]&0x7f) << 6) | (uint16(rawPacket[14]&0xfc) >> 2))
		packet.Temps[1] = c.fromRawTemp((uint16(rawPacket[14]&0x03) << 11) | (uint16(rawPacket[15]&0xff) << 3) | uint16(rawPacket[16]&0xe0)>>5)
		packet.Temps[0] = c.fromRawTemp((uint16(rawPacket[16]&0x1f) << 8) | uint16(rawPacket[17]&0xff))
	}

	// See https://github.com/combustion-inc/combustion-documentation/blob/main/probe_ble_specification.rst#battery-status-and-virtual-sensors
	packet.BatteryOK = (rawPacket[19] & 0x01) == 0x00

	// See https://github.com/combustion-inc/combustion-documentation/blob/main/probe_ble_specification.rst#virtual-sensors
	packet.VirtualCoreIndex = (rawPacket[19] >> 1) & 0x07          // bits 2-4 => core
	packet.VirtualSurfaceIndex = ((rawPacket[19] >> 4) & 0x03) + 3 // bits 5-6 => surface
	packet.VirtualAmbientIndex = ((rawPacket[19] >> 6) & 0x03) + 4 // bits 7-8 => ambient

	// See https://github.com/combustion-inc/combustion-documentation/blob/main/probe_ble_specification.rst#overheating-sensors
	var ohMask byte = 0x80
	for i := range 8 {
		packet.Overheating[7-i] = rawPacket[21]&ohMask == ohMask
		ohMask = ohMask >> 1
	}

	return &packet
}

func (c *Combustion) onScan(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
	md := device.AdvertisementPayload.ManufacturerData()
	if len(md) > 0 && md[0].CompanyID == CombustionManufacuterID {
		packet := c.ExtractCombustionPacket(md[0].Data)

		if c.packetHandler != nil {
			c.packetHandler(c, *packet)
		}
	}
}

func (c *Combustion) StartMonitoring(callback func(*Combustion, CombustionPacket)) error {
	c.packetHandler = callback

	if c.BluetoothAdapter == nil {
		c.BluetoothAdapter = bluetooth.DefaultAdapter
	}

	if err := c.BluetoothAdapter.Enable(); err != nil {
		return err
	}

	if err := c.BluetoothAdapter.Scan(c.onScan); err != nil {
		return err
	}

	return nil
}

func (c *Combustion) StopMonitoring() error {
	if c.BluetoothAdapter != nil {
		return c.BluetoothAdapter.StopScan()
	}
	return nil
}
