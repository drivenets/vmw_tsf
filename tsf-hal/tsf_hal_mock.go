package main

import (
	"fmt"
	"net"
	"sync"
)

type DnHalImpl struct {
	mutex       sync.Mutex
	initialized bool
}

var hal = &DnHalImpl{}

func NewDnHalMock() DnHal {
	hal.mutex.Lock()
	defer hal.mutex.Unlock()
	if !hal.initialized {
		hal.Init()
	}
	return hal
}

func (hal *DnHalImpl) Init() {
	hal.initialized = true
}

// type FlowSteeringTable map[Ipv4FlowKey]Ipv4Addr

func (*DnHalImpl) Steer(fk *FlowKey, nh net.IP) error {
	fmt.Printf("steer flow: %s to next hop: %s\n", fk, nh)
	return nil
}

var itl = map[IfName]InterfaceTelemetry{
	"halo1": {
		Speed:   10,
		RxBytes: 1000,
		RxBps:   100,
		TxBytes: 2000,
		TxBps:   200,
		Links: map[string]LinkTelemetry{
			"pod-b": {
				Delay:  15.33,
				Jitter: 4.93,
			},
			"pod-c": {
				Delay:  30.84,
				Jitter: 6.57,
			},
		},
	},
	"halo2": {
		Speed:   11,
		RxBytes: 1100,
		RxBps:   110,
		TxBytes: 2100,
		TxBps:   210,
		Links: map[string]LinkTelemetry{
			"pod-b": {
				Delay:  18.23,
				Jitter: 7.17,
			},
			"pod-c": {
				Delay:  20.91,
				Jitter: 8.22,
			},
		},
	},
}

func (*DnHalImpl) GetInterfaces(v InterfaceVisitor) error {
	for ifc, tl := range itl {
		err := v(ifc, &tl)
		if err != nil {
			return err
		}
	}
	return nil
}

var ftl = map[*FlowKey]FlowTelemetry{
	{
		Protocol: TCP,
		SrcAddr:  net.ParseIP("10.10.0.2"),
		DstAddr:  net.ParseIP("10.11.0.2"),
		SrcPort:  8081,
		DstPort:  8082,
	}: {
		// Rate
		RxRatePps: 100,
		TxRatePps: 200,
		RxRateBps: 1000,
		TxRateBps: 1000,

		// Total
		RxTotalPkts:  1000,
		TxTotalPkts:  2000,
		RxTotalBytes: 10000,
		TxTotalBytes: 20000,

		// Interfaces
		IngressIf: "halo1",
		EgressIf:  "halo2",
	},
	{
		Protocol: TCP,
		SrcAddr:  net.ParseIP("10.10.0.3"),
		DstAddr:  net.ParseIP("10.11.0.3"),
		SrcPort:  8181,
		DstPort:  8182,
	}: {
		// Rate
		RxRatePps: 110,
		TxRatePps: 210,
		RxRateBps: 1100,
		TxRateBps: 1100,

		// Total
		RxTotalPkts:  1100,
		TxTotalPkts:  2100,
		RxTotalBytes: 11000,
		TxTotalBytes: 21000,

		// Interfaces
		IngressIf: "halo2",
		EgressIf:  "halo1",
	},
}

func (*DnHalImpl) GetFlows(v FlowVisitor) error {
	for fk, tl := range ftl {
		err := v(fk, &tl)
		if err != nil {
			return err
		}
	}
	return nil
}
