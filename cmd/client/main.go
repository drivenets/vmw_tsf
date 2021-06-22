package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	hal "github.com/drivenets/vmw_tsf/pkg/hal"
	"github.com/prometheus/common/log"
)

var monFlowsOpt bool
var monIfcOpt bool
var monInterval int
var flushSteerOpt bool
var steerFromOpt string
var steerToOpt string
var steerProtoOpt string
var steerNextHopOpt string
var aclRuleId int
var noClsOpt bool
var noTwampOpt bool
var batchOpt bool
var countOpt int

func handleSteer(h hal.DnHal) {

	if steerFromOpt == "" {
		return
	}

	if steerToOpt == "" {
		panic("missing flow destination")
	}

	var err error
	var port uint64

	fk := hal.FlowKey{}

	tokens := strings.Split(steerFromOpt, ":")
	if len(tokens) != 2 {
		panic("failed to parse flow source")
	}
	fk.SrcAddr = net.ParseIP(tokens[0])
	if port, err = strconv.ParseUint(tokens[1], 10, 16); err != nil {
		panic("failed to parse flow source port")
	}
	fk.SrcPort = uint16(port)

	tokens = strings.Split(steerToOpt, ":")
	if len(tokens) != 2 {
		panic("failed to parse flow source")
	}
	fk.DstAddr = net.ParseIP(tokens[0])
	if port, err = strconv.ParseUint(tokens[1], 10, 16); err != nil {
		panic("failed to parse flow source port")
	}
	fk.DstPort = uint16(port)

	switch steerProtoOpt {
	case "tcp":
		fk.Protocol = hal.TCP
	case "udp":
		fk.Protocol = hal.UDP
	default:
		panic("unexpected flow protocol")
	}

	//hal.SetAclRuleIndex(aclRuleId)
	if steerNextHopOpt == "" {
		err = h.RemoveSteer([]hal.FlowKey{fk})
	} else {
		log.Infof("steer %v to %s", fk, steerNextHopOpt)
		steerItem := hal.SteerItem{Rule: &fk, NextHop: steerNextHopOpt}
		err = h.Steer([]hal.SteerItem{steerItem})
		log.Infof("verifying rule %v to %s has been applied", fk, steerNextHopOpt)
		log.Infoln(h.GetSteerInterface([]hal.SteerItem{steerItem}))
	}
	if err != nil {
		panic(err)
	}

	//fk1 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("1.4.4.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk2 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("2.4.5.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk3 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("3.4.6.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk4 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("4.4.7.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//
	//var acl = []hal.SteerItem{
	//	{Rule: &fk1, NextHop: steerNextHopOpt},
	//	{Rule: &fk2, NextHop: steerNextHopOpt},
	//	{Rule: &fk3, NextHop: ""},
	//	{Rule: &fk4, NextHop: steerNextHopOpt}}
	//err = h.Steer(acl)
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(h.GetSteerInterface(acl))
	//
	//err = h.RemoveSteer([]hal.FlowKey{fk1, fk2, fk3, fk})
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(h.GetSteerInterface(acl))
}

func clearScreen() {
	if noClsOpt {
		return
	}
	fmt.Print("\033[H\033[2J")
}

func printInterfaces(h hal.DnHal) {
	fmt.Println()
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Interface\tType\tSpeed\tRx.Bytes/sec\tRx.Bytes/total\tTx.Bytes/sec\tTx.Bytes/total\tDelay\tJitter")
	fmt.Fprintln(w, "---------\t----\t-----\t------------\t--------------\t------------\t--------------\t-----\t------")
	count := 0
	ifType := ""
	getIfStats := func(ifc string, tm *hal.InterfaceTelemetry) error {
		count += 1
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%f\t%f\n",
			ifc, ifType, tm.Speed,
			tm.RxBps, tm.RxBytes,
			tm.TxBps, tm.TxBytes,
			tm.Link.Delay, tm.Link.Jitter)
		return nil
	}
	ifType = "lan"
	h.GetLanInterfaces(getIfStats)
	ifType = "wan"
	h.GetWanInterfaces(getIfStats)
	w.Flush()
	if count == 0 {
		fmt.Println("(none)")
	}
}

func printFlows(h hal.DnHal) {
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Source\tDestination\tProto\tIngress\tEgress\tPkt/sec\tPkt/total\tBytes/sec\tBytes/total")
	fmt.Fprintln(w, "------\t-----------\t-----\t-------\t------\t-------\t---------\t---------\t-----------")
	flows := make([]string, 0, 1024)
	h.GetFlows(
		func(key *hal.FlowKey, stat *hal.FlowTelemetry) error {
			flows = append(flows,
				fmt.Sprintf("%s:%d\t%s:%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n",
					key.SrcAddr, key.SrcPort,
					key.DstAddr, key.DstPort,
					key.Protocol,
					stat.IngressIf, stat.EgressIf,
					stat.RxRatePps, stat.RxTotalPkts,
					stat.RxRateBps, stat.RxTotalBytes))
			return nil
		})
	sort.Strings(flows)
	for _, fl := range flows {
		fmt.Fprint(w, fl)
	}
	w.Flush()
	if len(flows) == 0 {
		fmt.Println("(none)")
	}
}

func monitor(h hal.DnHal) {
	for {
		clearScreen()
		fmt.Println(time.Now())
		if monIfcOpt {
			printInterfaces(h)
		}
		if monFlowsOpt {
			printFlows(h)
		}
		time.Sleep(time.Duration(monInterval) * time.Second)
	}
}

type BatchInterfaceOutput struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Speed   uint64  `json:"speed"`
	RxBytes uint64  `json:"rx-bytes"`
	RxBps   uint64  `json:"rx-bps"`
	TxBytes uint64  `json:"tx-bytes"`
	TxBps   uint64  `json:"tx-bps"`
	Delay   float64 `json:"delay"`
	Jitter  float64 `json:"jitter"`
}

type BatchFlowOutput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Protocol    string `json:"protocol"`
	Ingress     string `json:"ingress"`
	Egress      string `json:"egress"`
	RxPps       uint64 `json:"rx-pps"`
	RxPkts      uint64 `json:"rx-packets"`
	RxBps       uint64 `json:"rx-bps"`
	RxBytes     uint64 `json:"rx-bytes"`
}

type BatchOutput struct {
	Time       float64                `json:"time"`
	Interfaces []BatchInterfaceOutput `json:"interfaces"`
	Flows      []BatchFlowOutput      `json:"flows"`
}

func jsonInterfaces(h hal.DnHal, b *BatchOutput) {
	ifType := "n/a"
	getIfStats := func(ifc string, tm *hal.InterfaceTelemetry) error {
		b.Interfaces = append(b.Interfaces, BatchInterfaceOutput{
			Name:    ifc,
			Type:    ifType,
			Speed:   tm.Speed,
			RxBps:   tm.RxBps,
			RxBytes: tm.RxBytes,
			TxBps:   tm.TxBps,
			TxBytes: tm.TxBytes,
			Delay:   tm.Link.Delay,
			Jitter:  tm.Link.Jitter,
		})
		return nil
	}
	ifType = "lan"
	h.GetLanInterfaces(getIfStats)
	ifType = "wan"
	h.GetWanInterfaces(getIfStats)
}

func jsonFlows(h hal.DnHal, b *BatchOutput) {
	h.GetFlows(
		func(key *hal.FlowKey, stat *hal.FlowTelemetry) error {
			b.Flows = append(b.Flows, BatchFlowOutput{
				Source:      fmt.Sprintf("%s:%d", key.SrcAddr, key.SrcPort),
				Destination: fmt.Sprintf("%s:%d", key.DstAddr, key.DstPort),
				Protocol:    key.Protocol.String(),
				Ingress:     stat.IngressIf,
				Egress:      stat.EgressIf,
				RxPps:       stat.RxRatePps,
				RxPkts:      stat.RxTotalPkts,
				RxBps:       stat.RxRateBps,
				RxBytes:     stat.RxTotalBytes,
			})
			return nil
		})
}

func batch(h hal.DnHal) {
	batch := make([]BatchOutput, countOpt)
	start := time.Now()
	for idx := 0; idx < countOpt; idx++ {
		time.Sleep(time.Duration(monInterval) * time.Second)
		entry := BatchOutput{
			Time:       time.Since(start).Seconds(),
			Interfaces: make([]BatchInterfaceOutput, 0, 3),
			Flows:      make([]BatchFlowOutput, 0, 1000)}
		if monIfcOpt {
			jsonInterfaces(h, &entry)
		}
		if monFlowsOpt {
			jsonFlows(h, &entry)
		}
		batch[idx] = entry
	}
	out, err := json.Marshal(batch)
	if err != nil {
		log.Error("Failed to marshal json output. Reason:", err)
	}
	fmt.Printf("%s\n", out)
}

func main() {
	flag.BoolVar(&monFlowsOpt, "flows", false, "monitor flows")
	flag.BoolVar(&monIfcOpt, "interfaces", false, "monitor interfaces")
	flag.IntVar(&monInterval, "interval", 10, "monitoring interval")
	flag.BoolVar(&flushSteerOpt, "flush", false, "flush old steering rules")
	flag.StringVar(&steerFromOpt, "from", "", "steer flow source ip:port")
	flag.StringVar(&steerToOpt, "to", "", "steer flow destination ip:port")
	flag.StringVar(&steerProtoOpt, "proto", "tcp", "steer flow protocol")
	flag.StringVar(&steerNextHopOpt, "next-hop", "", "steer to next-hop")
	flag.BoolVar(&noClsOpt, "nocls", false, "do not clear screen")
	flag.IntVar(&aclRuleId, "id", 10, "steer rule id")
	flag.BoolVar(&noTwampOpt, "notw", false, "do not start twamp measurements")
	flag.BoolVar(&batchOpt, "batch", false, "batch mode exit after first interval")
	flag.IntVar(&countOpt, "count", 1, "how many batch rounds to execute")
	flag.Parse()

	if noTwampOpt {
		os.Setenv("SKIP_TWAMP", "1")
	}
	if !(monFlowsOpt || monIfcOpt) {
		os.Setenv("SKIP_TWAMP", "1")
	}

	os.Setenv("IFC_SAMPLE", strconv.Itoa(monInterval))

	opts := make([]hal.OptionHal, 0, 1)
	if flushSteerOpt {
		opts = append(opts, hal.OptionHalFlushSteer())
	}
	h := hal.NewDnHal(opts...)
	handleSteer(h)
	if !(monFlowsOpt || monIfcOpt) {
		return
	}

	if batchOpt {
		batch(h)
	} else {
		monitor(h)
	}

}
