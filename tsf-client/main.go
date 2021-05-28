package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	hal "github.com/drivenets/vmw_tsf/tsf-hal"
)

func main() {
	h := hal.NewDnHal()

	for {
		fmt.Println()
		fmt.Println()
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "Interface\tSpeed\tRx.Bytes/sec\tRx.Bytes/total\tTx.Bytes/sec\tTx.Bytes/total\tDelay\tJitter")
		fmt.Fprintln(w, "---------\t-----\t------------\t--------------\t------------\t--------------\t-----\t------")
		count := 0
		h.GetInterfaces(
			func(ifc string, tm *hal.InterfaceTelemetry) error {
				count += 1
				fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%f\t%f\n",
					ifc, tm.Speed,
					tm.RxBps, tm.RxBytes,
					tm.TxBps, tm.TxBytes,
					tm.Link.Delay, tm.Link.Jitter)
				return nil
			})
		w.Flush()
		if count == 0 {
			fmt.Println("(none)")
		}

		fmt.Println()
		w = tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "Source\tDestination\tProto\tIngress\tEgress\tPkt/sec\tPkt/total\tBytes/sec\tBytes/total")
		fmt.Fprintln(w, "------\t-----------\t-----\t-------\t------\t-------\t---------\t---------\t-----------")
		count = 0
		h.GetFlows(
			func(key *hal.FlowKey, stat *hal.FlowTelemetry) error {
				count += 1
				fmt.Fprintf(w, "%s:%d\t%s:%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n",
					key.SrcAddr, key.SrcPort,
					key.DstAddr, key.DstPort,
					key.Protocol,
					stat.IngressIf, stat.EgressIf,
					stat.RxRatePps, stat.RxTotalPkts,
					stat.RxRateBps, stat.RxTotalBytes)
				return nil
			})
		w.Flush()
		if count == 0 {
			fmt.Println("(none)")
		}
		time.Sleep(10 * time.Second)
	}
}
