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
		"flow(proto=%v, saddr=%s, daddr=%s, sport=%d, dport=%d)",
		fk.Protocol, fk.SrcAddr, fk.DstAddr, fk.SrcPort, fk.DstPort)
}
