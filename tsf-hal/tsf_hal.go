package main

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

// Note interface naming needs a translation layer between NCP ports
// used by the SI and interfaces presented to the HALO container
type IfName string

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
	RxBytes uint64
	RxBps   uint64
	TxBytes uint64
	TxBps   uint64
	// Link telemetry reports ONLY information about nodes/neighbors
	// that are directly connected to us (derived from configuration)
	// Using "string" as key because net.IP is a slice
	Links map[string]LinkTelemetry
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
	IngressIf IfName
	EgressIf  IfName
}

type InterfaceVisitor func(IfName, *InterfaceTelemetry) error
type FlowVisitor func(*FlowKey, *FlowTelemetry) error

// HAL interface
type DnHal interface {
	Steer(*FlowKey, net.IP) error
	GetInterfaces(InterfaceVisitor) error
	GetFlows(FlowVisitor) error
}
