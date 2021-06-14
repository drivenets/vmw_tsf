package hal

import "net"

type FlowProto uint8

const (
	TCP = FlowProto(0x06)
	UDP = FlowProto(0x11)
)

type FlowKey struct {
	Protocol FlowProto
	SrcAddr  net.IP
	DstAddr  net.IP
	SrcPort  uint16
	DstPort  uint16
}

type SteerItem struct {
	Rule    *FlowKey
	NextHop string
}

// Note interface naming needs a translation layer between NCP ports
// used by the SI and interfaces presented to the HALO container
//type string string

// Currently we don't split delay and jitter depending on traffic
// direction: egress or ingress. Instead we're using aggregated
// round-trip values depending on measurement method
type LinkTelemetry struct {
	// assumption: focused on the TX
	Delay  float64
	Jitter float64
}

// Here we report statistics collected from the associated NCP port
// but this may not be correct if the port is also used by traffic
// other than HALO. As an alternative we could provide Linux kernel
// statistics for the virtual interface passed to the HALO container
// but this may not see all rx/tx in case of hardware offload
type InterfaceTelemetry struct {
	Speed   uint64
	RxBytes uint64 `json:"rx-octets"`
	RxBps   uint64 `json:"rx-bits-per-second"`
	TxBytes uint64 `json:"tx-octets"`
	TxBps   uint64 `json:"tx-bits-per-second"`
	// An interface is mapped 1:1 to a tunnel between two HALO
	// neighbors. The delay and jitter in this case refers to the
	// link represented by this interface.
	// (For LAN interfaces Delay and Jitter have no meaning)
	Link LinkTelemetry
}

// Note: Tx counters are currently not supported because of J2 limitations
type FlowTelemetry struct {
	// Rate
	RxRatePps uint64
	TxRatePps uint64
	RxRateBps uint64
	TxRateBps uint64

	// Total counters
	RxTotalPkts  uint64
	TxTotalPkts  uint64
	RxTotalBytes uint64
	TxTotalBytes uint64

	// Interfaces
	IngressIf string
	EgressIf  string
}

type InterfaceVisitor func(string, *InterfaceTelemetry) error
type FlowVisitor func(*FlowKey, *FlowTelemetry) error

// DnHal interface
type DnHal interface {
	Steer([]SteerItem) error
	RemoveSteer([]FlowKey) error
	GetSteerInterface([]SteerItem) []string
	GetInterfaces(InterfaceVisitor) error // WILL BE DEPRECATED (returns both LAN and WAN)
	GetLanInterfaces(InterfaceVisitor) error
	GetWanInterfaces(InterfaceVisitor) error
	GetFlows(FlowVisitor) error
}
