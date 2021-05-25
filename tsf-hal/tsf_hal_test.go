package hal

import (
	"fmt"
	"net"
	"testing"
)

func TestSteer(t *testing.T) {
	hal := NewDnHalMock()
	flow := FlowKey{
		Protocol: TCP,
		SrcAddr:  net.ParseIP("10.10.0.1"),
		DstAddr:  net.ParseIP("10.11.0.1"),
		SrcPort:  2038,
		DstPort:  443,
	}
	err := hal.Steer(&flow, "halo2")
	if err != nil {
		t.Errorf("Failed to steer flow: flow=%q, reason=%q", flow, err)
	}
}

func TestGetInterfaces(t *testing.T) {
	hal := NewDnHalMock()
	var count int
	hal.GetInterfaces(
		func(ifc IfName, tm *InterfaceTelemetry) error {
			count += 1
			fmt.Printf("interface: %s\n", ifc)
			fmt.Printf("   speed: %d\n", tm.Speed)
			fmt.Printf("   rx bytes total: %d\n", tm.RxBytes)
			fmt.Printf("   tx bytes total: %d\n", tm.TxBytes)
			fmt.Printf("   rx bytes/sec: %d\n", tm.RxBps)
			fmt.Printf("   tx bytes/sec: %d\n", tm.TxBps)
			fmt.Printf("   delay %.03f\n", tm.Link.Delay)
			fmt.Printf("   jitter %.03f\n", tm.Link.Jitter)
			return nil
		},
	)
	fmt.Printf("(%d interfaces)\n", count)
}

func TestGetFlows(t *testing.T) {
	hal := NewDnHalMock()
	var count int
	hal.GetFlows(
		func(fk *FlowKey, tm *FlowTelemetry) error {
			count += 1
			fmt.Printf("flow: %s\n", fk)
			fmt.Printf("   rx pkts/sec: %d\n", tm.RxRatePps)
			fmt.Printf("   tx pkts/sec: %d\n", tm.TxRatePps)
			fmt.Printf("   rx bytes/sec: %d\n", tm.RxRateBps)
			fmt.Printf("   tx bytes/sec: %d\n", tm.TxRateBps)
			fmt.Printf("   rx packets total: %d\n", tm.RxTotalPkts)
			fmt.Printf("   tx packets total: %d\n", tm.TxTotalPkts)
			fmt.Printf("   rx bytes total: %d\n", tm.RxTotalBytes)
			fmt.Printf("   tx bytes total: %d\n", tm.TxTotalBytes)
			fmt.Printf("   ingress: %s\n", tm.IngressIf)
			fmt.Printf("   egress: %s\n", tm.EgressIf)
			return nil
		},
	)
	fmt.Printf("(%d flows)\n", count)
}
