package hal

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
)

func (p FlowProto) String() string {
	m := map[FlowProto]string{
		TCP: "tcp(0x06)",
		UDP: "udp(0x11)",
	}
	s, ok := m[p]
	if ok {
		return s
	}
	return strconv.Itoa(int(p))
}

func (fk *FlowKey) String() string {
	return fmt.Sprintf(
		"%s:%d -> %s:%d (%s)",
		fk.SrcAddr, fk.SrcPort, fk.DstAddr, fk.DstPort,
		fk.Protocol)
}

func (fm *FlowTelemetry) String() string {
	return fmt.Sprintf(
		"stats(interfaces: in=%s, out=%s; total: rx_bytes=%d, rx_pkts=%d, tx_bytes=%d, tx_pkts=%d; rate: rx_bps=%d, rx_pps=%d, tx_bps=%d, tx_pps=%d",
		fm.IngressIf, fm.EgressIf,
		fm.RxTotalBytes, fm.RxTotalPkts, fm.TxTotalBytes, fm.TxTotalPkts,
		fm.RxRateBps, fm.RxRatePps, fm.TxRateBps, fm.TxRatePps,
	)
}

type FlowsDebugger struct {
	flows []string
}

func NewFlowsDebugger() *FlowsDebugger {
	dbg := &FlowsDebugger{}
	dbg.flows = make([]string, 0, 1024)
	return &FlowsDebugger{}
}

func (dbg *FlowsDebugger) Print() {
	tmpf, err := os.OpenFile("/tmp/flows.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	w := tabwriter.NewWriter(tmpf, 1, 1, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "Source\tDestination\tProto\tIngress\tEgress\tPkt/sec\tPkt/total\tBytes/sec\tBytes/total\t")
	fmt.Fprintln(w, "------\t-----------\t-----\t-------\t------\t-------\t---------\t---------\t-----------\t")

	sort.Strings(dbg.flows)
	for _, fl := range dbg.flows {
		fmt.Fprint(w, fl)
	}
	w.Flush()
	if len(dbg.flows) == 0 {
		fmt.Fprintln(tmpf, "(none)")
	}
	tmpf.Close()
}

func (dbg *FlowsDebugger) Flow(key *FlowKey, stat *FlowTelemetry) error {
	dbg.flows = append(dbg.flows,
		fmt.Sprintf("%s:%d\t%s:%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t\n",
			key.SrcAddr, key.SrcPort,
			key.DstAddr, key.DstPort,
			key.Protocol,
			stat.IngressIf, stat.EgressIf,
			stat.RxRatePps, stat.RxTotalPkts,
			stat.RxRateBps, stat.RxTotalBytes))
	return nil
}
