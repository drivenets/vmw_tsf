package hal

import (
	"fmt"
	"strconv"
)

func (p FlowProto) String() string {
	m := map[FlowProto]string{
		TCP: "tcp",
		UDP: "udp",
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
