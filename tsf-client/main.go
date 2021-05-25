package main

import (
	"fmt"

	"drivenets.com/tsf/hal"
)

func main() {
	h := hal.NewDnHalMock()
	var count int
	h.GetInterfaces(
		func(ifc hal.IfName, tm *hal.InterfaceTelemetry) error {
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
