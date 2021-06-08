package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	hal "github.com/drivenets/vmw_tsf/tsf-hal"
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

func handleSteer(h hal.DnHal) {

	if steerFromOpt == "" {
		return
	}

	if steerToOpt == "" {
		panic("missing flow destination")
	}

	if steerNextHopOpt == "" {
		panic("missint next hop")
	}

	var err error
	var port uint64

	fk := &hal.FlowKey{}

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

	fmt.Println("steer", fk, "to", steerNextHopOpt, "rule-id", aclRuleId)
	hal.SetAclRuleIndex(aclRuleId)
	err = h.Steer(fk, steerNextHopOpt)
	if err != nil {
		panic(err)
	}

	//fmt.Println("delete steer", fk, "to", steerNextHopOpt, "rule-id")
	//err = h.RemoveSteer(fk, steerNextHopOpt)
	//if err != nil {
	//	panic(err)
	//}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printInterfaces(h hal.DnHal) {
	if !monIfcOpt {
		return
	}
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
	if !monFlowsOpt {
		return
	}
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

func main() {
	flag.BoolVar(&monFlowsOpt, "flows", false, "monitor flows")
	flag.BoolVar(&monIfcOpt, "interfaces", false, "monitor interfaces")
	flag.IntVar(&monInterval, "interval", 10, "monitoring interval")
	flag.BoolVar(&flushSteerOpt, "flush", false, "flush old steering rules")
	flag.StringVar(&steerFromOpt, "from", "", "steer flow source ip:port")
	flag.StringVar(&steerToOpt, "to", "", "steer flow destination ip:port")
	flag.StringVar(&steerProtoOpt, "proto", "tcp", "steer flow protocol")
	flag.StringVar(&steerNextHopOpt, "next-hop", "", "steer to next-hop")
	flag.IntVar(&aclRuleId, "id", 10, "steer rule id")
	flag.Parse()

	if !(monFlowsOpt || monIfcOpt) {
		os.Setenv("SKIP_TWAMP", "1")
	}

	h := hal.NewDnHal(flushSteerOpt)
	handleSteer(h)
	if !(monFlowsOpt || monIfcOpt) {
		return
	}

	os.Setenv("IFC_SAMPLE", strconv.Itoa(monInterval))
	for {
		clearScreen()
		fmt.Println(time.Now())
		printInterfaces(h)
		printFlows(h)
		time.Sleep(time.Duration(monInterval) * time.Second)
	}
}
