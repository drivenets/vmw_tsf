package hal

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	stdLog "log"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/Juniper/go-netconf/netconf"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"

	"os"
	"strconv"
	"sync"
	"text/template"
	"time"

	gfu "github.com/cloudflare/goflow/v3/utils"
	log "github.com/sirupsen/logrus"

	flowmessage "github.com/cloudflare/goflow/v3/pb"

	pb "github.com/drivenets/vmw_tsf/pkg/hal/proto"
	twamp "github.com/drivenets/vmw_tsf/pkg/twamp"
)

type FlowAggregate struct {
	key   *FlowKey
	inIf  string
	outIf string

	packets uint64
	bytes   uint64
}

func (agg *FlowAggregate) ToTelemetry(interval time.Duration) *FlowTelemetry {
	millis := uint64(interval.Milliseconds())
	if millis < 1 {
		millis = 1
	}
	return &FlowTelemetry{
		// Rate
		RxRatePps: (((agg.packets + 1) * 1000) - 1) / millis,
		TxRatePps: 0,
		RxRateBps: (((agg.bytes + 1) * 1000) - 1) / millis,
		TxRateBps: 0,

		// Total counters
		RxTotalPkts:  agg.packets,
		TxTotalPkts:  0,
		RxTotalBytes: agg.bytes,
		TxTotalBytes: 0,

		// Interfaces
		IngressIf: agg.inIf,
		EgressIf:  agg.outIf,
	}
}

const NETFLOW_ID_INVALID = 0xFFFFFFFF

type Interface struct {
	Upper     string
	Lower     string
	NetFlowId uint32
	NextHop   net.IP
	Stats     InterfaceTelemetry
	Twamp     struct {
		Peer string
		Port uint16
	}
}

type DnHalImpl struct {
	mutex       sync.Mutex
	initialized bool
	initOptions struct {
		flushSteer  bool
		statsServer bool
	}
	grpcAddr   string
	interfaces struct {
		UpdateInterval time.Duration
		Map            struct {
			Lock        sync.RWMutex
			UpperSorted struct {
				Lan []string
				Wan []string
			}
			Upper2Interface   map[string]*Interface
			Lower2Interface   map[string]*Interface
			NetFlow2Interface map[uint32]*Interface
			NextHop2Interface map[string]*Interface
		}
	}
	flows struct {
		state     *gfu.StateNetFlow
		start     time.Time
		aggregate map[string]*FlowAggregate
	}
	aclRules map[int]string
	debug    bool
}

var hal = &DnHalImpl{}

type OptionHal func(*DnHalImpl) error

func OptionHalFlushSteer() OptionHal {
	return func(hal *DnHalImpl) error {
		hal.initOptions.flushSteer = true
		return nil
	}
}

func OptionHalStatsServer() OptionHal {
	return func(hal *DnHalImpl) error {
		hal.initOptions.statsServer = true
		return nil
	}
}

func NewDnHal(options ...OptionHal) DnHal {
	for _, op := range options {
		err := op(hal)
		if err != nil {
			log.Fatalf("Failed to setup HAL instance. Reason: %v", err)
		}
	}

	if !hal.initialized {
		hal.Init()
	}
	return hal
}

const DRIVENETS_DNOS_ADDR = "localhost"

func (hal *DnHalImpl) InitFlows() {
	hal.flows.state = &gfu.StateNetFlow{
		Transport: hal,
		Logger:    log.StandardLogger(),
	}

	hal.flows.start = time.Now()
	hal.flows.aggregate = make(map[string]*FlowAggregate)
}

func (hal *DnHalImpl) InitNetConf() {
	log.Info("Initializing netconf client")
	session := NetConfConnector()

	xmlString, err := getAclRuleFilterXml()
	if err != nil {
		log.Errorf("Failed to prepare an XML struct, err: %v", err)
		panic(err)
	}

	data, err := session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		log.Errorf("Failed to retrieve ACL rules, err: %v", err)
		panic(err)
	}

	var response Data
	if err := xml.Unmarshal([]byte(data.Data), &response); err != nil {
		panic(err)
	}

	protocols := map[string]FlowProto{"tcp(0x06)": TCP, "udp(0x11)": UDP}
	maxID := accessListID
	for _, v := range response.DrivenetsTopReply.AccessListsDnAccessControlListReply.Ipv4.AccessList.Rules.Rule {
		if v.RuleId == DefaultRuleId {
			continue
		}
		srcIpv4Addr, _, err := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.SourceIpv4)
		if err != nil {
			log.Warn(v, err)
			continue
		}

		DstIpv4Addr, _, err := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.DestinationIpv4)
		if err != nil {
			log.Warn(err)
			continue
		}

		fk := FlowKey{
			Protocol: protocols[v.RuleConfigItems.Protocol],
			SrcAddr:  srcIpv4Addr,
			DstAddr:  DstIpv4Addr,
			SrcPort:  v.RuleConfigItems.Matches.L4AclMatch.SourcePortRange.LowerPort,
			DstPort:  v.RuleConfigItems.Matches.L4AclMatch.DestinationPortRange.LowerPort,
		}
		log.Infof("Loading rule to cache - id: %d, rule: %s, next-hop: %v", v.RuleId, fk.AsKey(), v.RuleConfigItems.Nexthop1)
		hal.AclRuleCacheAdd(fk, v.RuleId)
		if v.RuleId > maxID {
			accessListID = v.RuleId + 10
		}
	}
	log.Debugf("accessListID: %d", accessListID)
}

func (hal *DnHalImpl) Init() {
	if _, ok := os.LookupEnv("DEBUG"); ok {
		log.SetLevel(log.DebugLevel)
		hal.debug = true
	}
	hal.mutex.Lock()
	defer hal.mutex.Unlock()

	var ok bool
	if hal.grpcAddr, ok = os.LookupEnv("DNOS_ADDR"); !ok {
		hal.grpcAddr = DRIVENETS_DNOS_ADDR
	}
	hal.grpcAddr = hal.grpcAddr + ":50051"
	hal.aclRules = make(map[int]string)
	hal.InitInterfaces()

	if hal.initOptions.flushSteer {
		err := SteeringAclCleanup()
		if err != nil {
			log.Errorf("Failed to cleanup old access lists. Reason: %v", err)
		}
	}
	go monitorInterfaces()

	hal.InitFlows()
	hal.InitNetConf()

	go monitorFlows()

	hal.initialized = true

	if hal.initOptions.statsServer {
		go statsServer(hal)
	}
}

const HALO_INTERFACES_COUNT = 2
const HALO_ACL_BUCKET_NAME = "Steering"

type OptionInterface func(*Interface) error

func OptionInterfaceUpper(name string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.Upper = name
		return nil
	}
}

func OptionInterfaceLower(name string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.Lower = name
		return nil
	}
}

func OptionInterfaceNetFlowId(nfid uint32) OptionInterface {
	return func(ifc *Interface) error {
		ifc.NetFlowId = nfid
		return nil
	}
}

func OptionInterfaceUpdateNetFlowId(client gnmi.GNMIClient) OptionInterface {
	return func(ifc *Interface) error {
		var err error

		if ifc.NetFlowId, err = getDnIfIndex(client, ifc.Lower); err != nil {
			log.Errorf("Failed to get interface index for %s", ifc.Lower)
			return err
		}
		return nil
	}
}

func OptionInterfaceNextHop(nh string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.NextHop = net.ParseIP(nh)
		return nil
	}
}

func OptionInterfaceTwamp(peer string, port string) func(*Interface) error {
	return func(ifc *Interface) error {
		var port64 uint64
		port64, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			log.Error("Failed to parse TWAMP port %d", port)
			return err
		}
		port16 := uint16(port64)

		ifc.Twamp.Peer = peer
		ifc.Twamp.Port = port16
		return nil
	}
}

func (ifc *Interface) IsWan() bool {
	wan := true
	if ip4 := ifc.NextHop.To4(); ip4 != nil {
		if ifc.NextHop.Equal(net.IPv4zero) {
			wan = false
		}
	} else if ifc.NextHop.Equal(net.IPv6zero) {
		wan = false
	}
	return wan
}

func NewInterface(options ...OptionInterface) (*Interface, error) {
	ifc := &Interface{
		NextHop:   net.ParseIP("0.0.0.0"),
		NetFlowId: NETFLOW_ID_INVALID,
	}
	for _, op := range options {
		err := op(ifc)
		if err != nil {
			return nil, err
		}
	}
	hal.interfaces.Map.Lock.Lock()
	defer hal.interfaces.Map.Lock.Unlock()

	if _, ok := hal.interfaces.Map.Upper2Interface[ifc.Upper]; ok {
		return nil, fmt.Errorf("interface %s already exists", ifc.Upper)
	}

	hal.interfaces.Map.Lower2Interface[ifc.Lower] = ifc
	hal.interfaces.Map.Upper2Interface[ifc.Upper] = ifc
	if ifc.NetFlowId != NETFLOW_ID_INVALID {
		hal.interfaces.Map.NetFlow2Interface[ifc.NetFlowId] = ifc
	}
	if ifc.IsWan() {
		hal.interfaces.Map.NextHop2Interface[ifc.Upper] = ifc
		hal.interfaces.Map.UpperSorted.Wan = append(
			hal.interfaces.Map.UpperSorted.Wan, ifc.Upper)
		sort.Strings(hal.interfaces.Map.UpperSorted.Wan)
	} else {
		hal.interfaces.Map.UpperSorted.Lan = append(
			hal.interfaces.Map.UpperSorted.Lan, ifc.Upper)
		sort.Strings(hal.interfaces.Map.UpperSorted.Lan)
	}
	if monitor != nil {
		monitor.Add(ifc.Lower)
	}
	return ifc, nil
}

func remove(haystack []string, needle string) []string {
	for i, item := range haystack {
		if item == needle {
			haystack = append(haystack[:i], haystack[i+1:]...)
			break
		}
	}
	return haystack
}

func RemoveInterface(upper string) error {
	var ifc *Interface
	var ok bool

	hal.interfaces.Map.Lock.Lock()
	defer hal.interfaces.Map.Lock.Unlock()

	ifc, ok = hal.interfaces.Map.Upper2Interface[upper]
	if !ok {
		log.Infof("skip remove interface %s. Not found", upper)
		return nil
	}

	log.Infof("remove interface %s", upper)
	delete(hal.interfaces.Map.Lower2Interface, ifc.Lower)
	delete(hal.interfaces.Map.Upper2Interface, ifc.Upper)
	if ifc.NetFlowId != NETFLOW_ID_INVALID {
		delete(hal.interfaces.Map.NetFlow2Interface, ifc.NetFlowId)
	}
	delete(hal.interfaces.Map.NextHop2Interface, ifc.Upper)
	hal.interfaces.Map.UpperSorted.Wan = remove(
		hal.interfaces.Map.UpperSorted.Wan, ifc.Upper)
	hal.interfaces.Map.UpperSorted.Lan = remove(
		hal.interfaces.Map.UpperSorted.Lan, ifc.Upper)
	if monitor != nil {
		if err := monitor.Remove(ifc.Lower); err != nil {
			return fmt.Errorf("failed to remove interface monitor %s", ifc.Lower)
		}
	}
	return nil
}

func (hal *DnHalImpl) InitInterfaces() {
	var haloIf string
	var dnIf string
	var peer string
	var port string
	var ok bool

	hal.interfaces.Map.UpperSorted.Lan = make([]string, 0)
	hal.interfaces.Map.UpperSorted.Wan = make([]string, 0)
	hal.interfaces.Map.Upper2Interface = make(map[string]*Interface)
	hal.interfaces.Map.Lower2Interface = make(map[string]*Interface)
	hal.interfaces.Map.NetFlow2Interface = make(map[uint32]*Interface)
	hal.interfaces.Map.NextHop2Interface = make(map[string]*Interface)

	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatal("failed to connect to gRPC server: %s. Reason: %w", hal.grpcAddr, err)
	}
	defer conn.Close()
	client := gnmi.NewGNMIClient(conn)

	sys_interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("Failed to list network interfaces: %v", err)
	}

	for _, ifc := range sys_interfaces {
		if ifc.Name != "halo_local0" {
			continue
		}
		if dnIf, ok = os.LookupEnv("HALO_LOCAL_IFACE"); !ok {
			log.Fatal("Missing DN interface for HALO_LOCAL")
		}
		_, err := NewInterface(
			OptionInterfaceUpper(ifc.Name),
			OptionInterfaceLower(dnIf),
			OptionInterfaceUpdateNetFlowId(client),
		)
		if err != nil {
			log.Fatal("Failed to create local interface. Reason:", err)
		}
		break
	}

	var sample string
	var interval float64 = 1
	if sample, ok = os.LookupEnv("IFC_SAMPLE"); ok {
		var err error
		if interval, err = strconv.ParseFloat(sample, 64); err != nil {
			log.Fatalf("Failed to parse interface sampling interval: %v", interval)
		}
	}
	hal.interfaces.UpdateInterval = time.Duration(interval * float64(time.Second))

	for _, ifc := range sys_interfaces {
		if !strings.HasPrefix(ifc.Name, "halo") {
			continue
		}
		if strings.HasPrefix(ifc.Name, "halo_local") {
			continue
		}
		var idx int
		if _, err = fmt.Sscanf(ifc.Name, "halo%d", &idx); err != nil {
			log.Warnf("failed to parse interface index %s. Skip it", ifc.Name)
		}
		if dnIf, ok = os.LookupEnv(fmt.Sprintf("HALO%d_IFACE", idx)); !ok {
			dnIf = fmt.Sprintf("ge100-0/0/%d", idx)
		}

		var nextHop1 string
		if nextHop1, ok = os.LookupEnv(fmt.Sprintf("HALO%d_NEXT_HOP", idx)); !ok {
			log.Fatalf("Can not find nexthop in HALO%d_IFACE", idx)
		}

		if peer, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP", idx)); !ok {
			peer = "0.0.0.0:862"
			log.Error("Failed to get TWAMP server for: ", dnIf)
		}
		if port, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP_PORT", idx)); !ok {
			port = "10001"
			log.Error("Failed to get TWAMP server UDP ports for: ", dnIf)
		}

		haloIf = fmt.Sprintf("halo%d", idx)
		_, err := NewInterface(
			OptionInterfaceUpper(haloIf),
			OptionInterfaceLower(dnIf),
			OptionInterfaceTwamp(peer, port),
			OptionInterfaceNextHop(nextHop1),
			OptionInterfaceUpdateNetFlowId(client),
		)
		if err != nil {
			log.Fatal("Failed to create interface. Reason:", err)
		}
	}
}

const DRIVENETS_IFINDEX_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/if-index"

func getDnIfIndex(client gnmi.GNMIClient, lower string) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var err error

	var pathList []*gnmi.Path
	idxPath := fmt.Sprintf(DRIVENETS_IFINDEX_PATH_TEMPLATE, lower)
	pbPath, err := ygot.StringToPath(idxPath, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		return 0, fmt.Errorf("failed to convert %q to gNMI Path. Reason: %w", idxPath, err)
	}
	pathList = append(pathList, pbPath)
	resp, err := client.Get(ctx, &gnmi.GetRequest{
		Path:     pathList,
		Encoding: gnmi.Encoding_JSON,
	})
	if err != nil {
		return 0, fmt.Errorf("request failed for path %s", pathList)
	}
	notifs := resp.GetNotification()
	if len(notifs) != 1 {
		return 0, fmt.Errorf("unexpected notifications count: want 1, got %d, path=%s", len(notifs), idxPath)
	}
	updates := notifs[0].GetUpdate()
	if len(updates) != 1 {
		return 0, fmt.Errorf("unexpected updates count: want 1, got %d, path=%s", len(updates), idxPath)
	}
	val := updates[0].GetVal()
	var ifIndex uint32
	if err := json.Unmarshal(val.GetJsonVal(), &ifIndex); err != nil {
		log.Fatalf("failed to convert value: %q. Reason: %v", val, err)
	}
	return ifIndex, nil
}

const DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/counters/ethernet-counters"
const DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/interface-speed"

func interfaceOperPaths(lower string) (*gnmi.Path, *gnmi.Path) {
	speed, _ := ygot.StringToPath(
		fmt.Sprintf(DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE, lower),
		ygot.StructuredPath, ygot.StringSlicePath)
	counters, _ := ygot.StringToPath(
		fmt.Sprintf(DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE, lower),
		ygot.StructuredPath, ygot.StringSlicePath)
	return speed, counters
}

func copyLanInterfaces() []*Interface {
	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	wan := make([]*Interface, len(hal.interfaces.Map.UpperSorted.Lan))
	for idx, ifc := range hal.interfaces.Map.UpperSorted.Lan {
		wan[idx] = hal.interfaces.Map.Upper2Interface[ifc]
	}
	return wan
}

func copyWanInterfaces() []*Interface {
	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	wan := make([]*Interface, len(hal.interfaces.Map.UpperSorted.Wan))
	for idx, ifc := range hal.interfaces.Map.UpperSorted.Wan {
		wan[idx] = hal.interfaces.Map.Upper2Interface[ifc]
	}
	return wan
}

func findInterfaceByLower(lower string) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.Lower2Interface[lower]; ok {
		return ifc
	}
	return nil
}

func findInterfaceByNetFlowId(nfid uint32) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.NetFlow2Interface[nfid]; ok {
		return ifc
	}
	return nil
}

func findInterfaceByNextHop(nh string) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.NextHop2Interface[nh]; ok {
		return ifc
	}
	return nil
}

func updateInterfaceDelayJitter(upper string, delay float64, jitter float64) {
	hal.interfaces.Map.Lock.Lock()
	defer hal.interfaces.Map.Lock.Unlock()

	if ifc, ok := hal.interfaces.Map.Upper2Interface[upper]; ok {
		ifc.Stats.Link.Delay = delay
		ifc.Stats.Link.Jitter = jitter
	}
}

var monitor *InterfaceMonitor

func monitorInterfaces() {
	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	monitor, err = NewInterfaceMonitor(hal.grpcAddr, hal.interfaces.UpdateInterval)
	if err != nil {
		log.Fatalf("Failed to start. Reason: %s", err)
	}

	for _, ifc := range copyLanInterfaces() {
		monitor.Add(ifc.Lower)
	}

	for _, ifc := range copyWanInterfaces() {
		monitor.Add(ifc.Lower)
	}

	twampTests := make(map[string]*twamp.TwampTest)

	var ok bool
	var skipTwamp string
	var twampConn *twamp.TwampConnection
	if skipTwamp, ok = os.LookupEnv("SKIP_TWAMP"); !ok {
		skipTwamp = "0"
	}

	if skipTwamp != "1" {

		for _, ifc := range copyWanInterfaces() {
			twampClient := twamp.NewClient()
			twampConn, err = twampClient.Connect(ifc.Twamp.Peer)
			if err != nil {
				log.Errorf("Failed to connect TWAMP %s. Reason: %s", ifc.Twamp.Peer, err)
				continue
			}
			defer twampConn.Close()

			twampSession, err := twampConn.CreateSession(
				twamp.TwampSessionConfig{
					SenderPort:   int(ifc.Twamp.Port),
					ReceiverPort: int(ifc.Twamp.Port) + 1,
					Timeout:      1,
					Padding:      42,
					TOS:          0,
				},
			)
			if err != nil {
				log.Error(err)
				continue
			}
			defer twampSession.Stop()

			twampTest, ok := twampSession.CreateTest()
			if ok != nil {
				log.Error(ok)
				continue
			}
			twampTests[ifc.Upper] = twampTest
		}
	}

	djt := time.NewTicker(hal.interfaces.UpdateInterval)
	for {
		select {
		case u := <-monitor.Speed():
			if ifc := findInterfaceByLower(u.Name); ifc == nil {
				log.Warnf("No such interface %s. Skip speed update: %v", u.Name, u.Stats)
				continue
			} else {
				ifc.Stats.Speed = u.Stats.Speed
			}
		case u := <-monitor.Stats():
			if ifc := findInterfaceByLower(u.Name); ifc == nil {
				log.Warnf("No such interface %s. Skip speed update: %v", u.Name, u.Stats)
				continue
			} else {
				ifc.Stats.RxBytes = u.Stats.RxBytes
				ifc.Stats.RxBps = u.Stats.RxBps
				ifc.Stats.TxBytes = u.Stats.TxBytes
				ifc.Stats.TxBps = u.Stats.TxBps
			}
		case <-djt.C:
			if skipTwamp != "1" {
				for upper, tt := range twampTests {
					results := tt.RunX(5)
					updateInterfaceDelayJitter(upper,
						float64(results.Stat.Avg)/float64(time.Millisecond),
						float64(results.Stat.Jitter)/float64(time.Millisecond),
					)
				}
			}
		}
	}
}

func monitorFlows() {
	err := hal.flows.state.FlowRoutine(1, "0.0.0.0", 2055, true)
	if err != nil {
		log.Fatalf("Fatal error: could not monitor flows (%v)", err)
	}
}

func (hal *DnHalImpl) GetInterfaces(v InterfaceVisitor) error {
	var err error
	if err = hal.GetLanInterfaces(v); err != nil {
		return err
	}
	if err = hal.GetWanInterfaces(v); err != nil {
		return err
	}
	return nil
}

func (hal *DnHalImpl) GetLanInterfaces(v InterfaceVisitor) error {
	for _, ifc := range copyLanInterfaces() {
		err := v(ifc.Upper, &ifc.Stats)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hal *DnHalImpl) GetWanInterfaces(v InterfaceVisitor) error {
	for _, ifc := range copyWanInterfaces() {
		err := v(ifc.Upper, &ifc.Stats)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fk *FlowKey) AsKey() string {
	return fmt.Sprintf(
		"%d:%s:%s:%d:%d", fk.Protocol,
		fk.SrcAddr, fk.DstAddr,
		fk.SrcPort, fk.DstPort)
}

func (hal *DnHalImpl) Publish(update []*flowmessage.FlowMessage) {
	for _, msg := range update {
		fk := &FlowKey{
			Protocol: FlowProto(msg.Proto),
			SrcAddr:  msg.SrcAddr,
			DstAddr:  msg.DstAddr,
			SrcPort:  uint16(msg.SrcPort),
			DstPort:  uint16(msg.DstPort),
		}

		var inIf string
		var outIf string
		var ok bool

		if ifc := findInterfaceByNetFlowId(msg.InIf); ifc == nil {
			inIf = "N/A"
		} else {
			inIf = ifc.Upper
		}
		if ifc := findInterfaceByNetFlowId(msg.OutIf); ifc == nil {
			outIf = "N/A"
		} else {
			outIf = ifc.Upper
		}

		// Update flows aggregate
		var agg *FlowAggregate
		key := fk.AsKey()
		aggregate := hal.flows.aggregate
		if agg, ok = aggregate[key]; ok {
			agg.bytes += msg.Bytes
			agg.packets += msg.Packets
			// Override ingress/egress interfaces
			agg.inIf = inIf
			agg.outIf = outIf
		} else {
			agg = &FlowAggregate{
				key:     fk,
				inIf:    inIf,
				outIf:   outIf,
				bytes:   msg.Bytes,
				packets: msg.Packets,
			}
			aggregate[key] = agg
		}
	}
}

func (*DnHalImpl) GetFlows(v FlowVisitor) error {
	aggregate := make(map[string]*FlowAggregate)
	// Swap current active with empty one
	aggregate, hal.flows.aggregate = hal.flows.aggregate, aggregate

	now, prev := time.Now(), hal.flows.start
	hal.flows.start = now
	if hal.debug {
		dbg := NewFlowsDebugger()
		for _, agg := range aggregate {
			stats := agg.ToTelemetry(now.Sub(prev))
			dbg.Flow(agg.key, stats)
			v(agg.key, stats)
		}
		dbg.Print()
	} else {
		for _, agg := range aggregate {
			stats := agg.ToTelemetry(now.Sub(prev))
			v(agg.key, stats)
		}

	}
	return nil
}

func SetAclRuleIndex(idx int) {
	accessListID = idx
}

func commitChanges() error {
	session := NetConfConnector()

	log.Info("Committing changes")
	_, err := session.Exec(netconf.RawMethod(Commit))
	if err != nil {
		if strings.Contains(err.Error(), "Commit failed: empty commit") {
			log.Println(err.Error())
		} else {
			return err
		}
	}

	return nil
}

func getAclRuleFilterXml() ([]byte, error) {
	conf := GetConfig{
		Filter: &Filter{
			DrivenetsTop: &DrivenetsTop{
				AttrXmlnsdnAccessControlList: "http://drivenets.com/ns/yang/dn-access-control-list",
				AccessListsDnAccessControlList: AccessListsDnAccessControlList{
					Ipv4: &Ipv4{
						AccessList: AccessList{Name: HALO_ACL_BUCKET_NAME},
					},
				},
			},
		},
		Source: &Source{RunningConfig: &RunningConfig{}},
	}

	return xml.MarshalIndent(conf, "", "    ")
}

func getAclRuleConfigXml(rules []Rule) ([]byte, error) {
	conf := EditConfig{
		Config: Config{
			DrivenetsTop: DrivenetsTop{
				AttrXmlnsdnAccessControlList: "http://drivenets.com/ns/yang/dn-access-control-list",
				AccessListsDnAccessControlList: AccessListsDnAccessControlList{
					Ipv4: &Ipv4{
						AccessList: AccessList{
							ConfigItems: &ConfigItems{Name: HALO_ACL_BUCKET_NAME},
							Name:        HALO_ACL_BUCKET_NAME,
							Rules:       Rules{Rule: rules},
						},
					},
				},
			},
		},
		TargetCandidate: TargetCandidate{Candidate: Candidate{}},
	}
	return xml.MarshalIndent(conf, "", "    ")
}

func getAclRulesDeleteXml() ([]byte, error) {
	conf := EditConfig{
		Config: Config{
			DrivenetsTop: DrivenetsTop{
				AttrXmlnsdnAccessControlList: "http://drivenets.com/ns/yang/dn-access-control-list",
				AccessListsDnAccessControlList: AccessListsDnAccessControlList{
					Ipv4: &Ipv4{
						AccessList: AccessList{
							Name:  HALO_ACL_BUCKET_NAME,
							Rules: Rules{AttrNcSpaceoperation: "delete"},
						},
					},
				},
			},
		},
		TargetCandidate: TargetCandidate{Candidate: Candidate{}},
	}
	return xml.MarshalIndent(conf, "", "    ")
}

func (hal *DnHalImpl) Steer(rules []SteerItem) error {
	session := NetConfConnector()
	ruleIdxMap := make(map[int]string)
	rulesList := make([]Rule, 0, len(rules))

	for _, rule := range rules {
		fk := rule.Rule
		nh := rule.NextHop
		var nextHop1 net.IP

		if nh != "" {
			ifc := findInterfaceByNextHop(nh)
			if ifc == nil {
				err := fmt.Errorf("no interface with next-hop: %s", nh)
				log.Warn(err)
				return err
			}
			nextHop1 = ifc.NextHop
		} else {
			nextHop1 = nil
		}

		ruleAsKey := fk.AsKey()
		currentRuleId := -1
		for k, v := range hal.aclRules {
			if v == ruleAsKey {
				currentRuleId = k
				log.Debugf("rule already exist with id, %d", currentRuleId)
				break
			}
		}
		if currentRuleId == -1 {
			currentRuleId = accessListID
			accessListID += 10
		}

		log.Infof("steer %v to %s, rule-id: %d", fk, nh, currentRuleId)
		ruleIdxMap[currentRuleId] = ruleAsKey
		rulesList = append(rulesList, Rule{
			RuleId: currentRuleId,
			RuleConfigItems: &RuleConfigItems{
				Ipv4Matches: &Ipv4Matches{
					DestinationIpv4: fmt.Sprintf("%s/32", fk.DstAddr),
					SourceIpv4:      fmt.Sprintf("%s/32", fk.SrcAddr),
				},
				Matches: &Matches{
					L4AclMatch: L4AclMatch{
						DestinationPortRange: DestinationPortRange{LowerPort: fk.DstPort},
						SourcePortRange:      SourcePortRange{LowerPort: fk.SrcPort},
					},
				},
				Nexthop1: &nextHop1,
				Protocol: fk.Protocol.String(),
				RuleType: "allow",
			},
		})
	}

	xmlString, err := getAclRuleConfigXml(rulesList)
	if err != nil {
		return err
	}

	_, err = session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		return err
	}

	err = commitChanges()
	if err != nil {
		return err
	}

	for k, v := range ruleIdxMap {
		hal.aclRules[k] = v
	}

	return nil
}

func (hal *DnHalImpl) RemoveSteer(rules []FlowKey) error {
	session := NetConfConnector()
	rulesToRemove := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		target := rule.AsKey()
		ruleId := -1
		for k, v := range hal.aclRules {
			if target == v {
				ruleId = k

				rulesToRemove = append(rulesToRemove, Rule{
					RuleId:               ruleId,
					AttrNcSpaceoperation: "delete",
				})
			}
		}
		log.Infof("removing %v, rule-id: %d", rule, ruleId)
		if ruleId == -1 {
			log.Warnf("rule %v not found in cache, skipping.", rule)
		}
	}

	if len(rulesToRemove) == 0 {
		log.Infof("nothing to remove..")
		return nil
	}

	xmlString, err := getAclRuleConfigXml(rulesToRemove)
	if err != nil {
		return err
	}

	_, err = session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		return err
	}

	err = commitChanges()
	if err != nil {
		return err
	}

	for _, idx := range rulesToRemove {
		delete(hal.aclRules, idx.RuleId)
	}

	return nil
}

func (h *DnHalImpl) GetSteerInterface(rules []SteerItem) (bool, []string) {
	log.Info("Retrieving ACL Steering rules")
	session := NetConfConnector()
	xmlString, err := getAclRuleFilterXml()
	if err != nil {
		log.Errorf("Failed to prepare an XML struct, err: %v", err)
		panic(err)
	}

	data, err := session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		log.Errorf("Failed to retrieve ACL rules, err: %v", err)
		panic(err)
	}

	var response Data
	if err := xml.Unmarshal([]byte(data.Data), &response); err != nil {
		panic(err)
	}

	protocols := map[string]FlowProto{"tcp(0x06)": TCP, "udp(0x11)": UDP}
	responseMap := make(map[string]string)
	for _, v := range response.DrivenetsTopReply.AccessListsDnAccessControlListReply.Ipv4.AccessList.Rules.Rule {
		srcIpv4Addr, _, _ := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.SourceIpv4)
		dstIpv4Addr, _, _ := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.DestinationIpv4)
		tmpItem := FlowKey{
			Protocol: protocols[v.RuleConfigItems.Protocol],
			SrcAddr:  srcIpv4Addr,
			DstAddr:  dstIpv4Addr,
			SrcPort:  v.RuleConfigItems.Matches.L4AclMatch.SourcePortRange.LowerPort,
			DstPort:  v.RuleConfigItems.Matches.L4AclMatch.DestinationPortRange.LowerPort,
		}

		if v.RuleConfigItems.Nexthop1 == nil {
			responseMap[tmpItem.AsKey()] = ""
		} else {
			for _, nh2iface := range h.interfaces.Map.NextHop2Interface {
				if nh2iface.NextHop.Equal(*v.RuleConfigItems.Nexthop1) {
					responseMap[tmpItem.AsKey()] = nh2iface.Upper
				}
			}
		}

	}

	ruleNxs := make([]string, 0, len(rules))
	matchSum := 0
	for _, rule := range rules {
		fk := rule.Rule
		if nh, exists := responseMap[fk.AsKey()]; exists && nh == rule.NextHop {
			matchSum += 1
			ruleNxs = append(ruleNxs, nh)
		} else {
			log.Infof("can not find rule %v on device", fk)
			ruleNxs = append(ruleNxs, "")
		}
	}

	status := matchSum == len(rules)
	return status, ruleNxs
}

const DefaultRuleId = 65434

func SteeringAclCleanup() error {
	session := NetConfConnector()

	log.Info("Removing all Steering rules")
	cleanRulesXML, err := getAclRulesDeleteXml()
	if err != nil {
		return err
	}
	_, err = session.Exec(netconf.RawMethod(cleanRulesXML))
	if err != nil {
		return err
	}

	log.Info("Adding default ACL rule")
	rulesList := []Rule{
		{
			RuleConfigItems: &RuleConfigItems{
				RuleType:    "allow",
				Protocol:    "any",
				Ipv4Matches: &Ipv4Matches{SourceIpv4: "any", DestinationIpv4: "any"},
				Matches: &Matches{
					L4AclMatch: L4AclMatch{
						SourcePortRange:      SourcePortRange{LowerPort: 0},
						DestinationPortRange: DestinationPortRange{LowerPort: 0},
					},
				},
			},
			RuleId: DefaultRuleId,
		},
	}
	xmlString, err := getAclRuleConfigXml(rulesList)
	if err != nil {
		return err
	}

	_, err = session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		return err
	}

	err = commitChanges()
	if err != nil {
		return err
	}

	return nil
}

var netconfSession *netconf.Session

func NetConfConnector() *netconf.Session {
	var err error
	var ok bool

	if _, ok = os.LookupEnv("DEBUG"); ok {
		defaultLog := stdLog.New(os.Stderr, "netconf ", 1)
		netconf.SetLog(netconf.NewStdLog(defaultLog, netconf.LogDebug))
	}

	if nc.netconfUser, ok = os.LookupEnv("NETCONF_USER"); !ok {
		nc.netconfUser = "dnroot"
	}
	if nc.netconfPassword, ok = os.LookupEnv("NETCONF_PASSWORD"); !ok {
		nc.netconfPassword = "dnroot"
	}
	if nc.netconfHost, ok = os.LookupEnv("DNOS_ADDR"); !ok {
		nc.netconfHost = DRIVENETS_DNOS_ADDR
	}

	if netconfSession == nil {
		sshConfig := &ssh.ClientConfig{
			User:            nc.netconfUser,
			Auth:            []ssh.AuthMethod{ssh.Password(nc.netconfPassword)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		netconfSession, err = netconf.DialSSH(nc.netconfHost, sshConfig)
		if err != nil {
			log.Fatal(err)
		}
	}
	//defer netconfSession.Close()
	return netconfSession
}

func (h *DnHalImpl) AclRuleCacheAdd(fk FlowKey, idx int) {
	h.aclRules[idx] = fk.AsKey()
}

func statsServer(hal *DnHalImpl) {
	lis, err := net.Listen("tcp", "0.0.0.0:7732")
	if err != nil {
		log.Warnf("Failed to listen on stats server port. Reason: %v", err)
		return
	}
	grpcServer := grpc.NewServer()
	pb.RegisterStatsServer(grpcServer, NewStatsServer(hal))
	pb.RegisterManagementServer(grpcServer, NewManagementServer(hal))
	log.Info("Starting gRPC server")
	grpcServer.Serve(lis)
}

const (
	rsvpTunnelAddXml = `<edit-config>
<target><candidate/></target>
<default-operation>merge</default-operation>
<error-option>rollback-on-error</error-option>
<config xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-protocol="http://drivenets.com/ns/yang/dn-protocol" xmlns:dn-rsvp="http://drivenets.com/ns/yang/dn-rsvp">
<dn-top:drivenets-top>
  <dn-protocol:protocols>
	<dn-rsvp:rsvp>
	  <tunnels>
		<tunnel>
		  <tunnel-name>{{.name}}</tunnel-name>
		  <primary>
			<config-items>
			  <cspf-calculation>disabled</cspf-calculation>
			</config-items>
			<path-options>
			  <path-option>
				<config-items>
				  <explicit-path-name>{{.name}}path</explicit-path-name>
				  <priority>1</priority>
				</config-items>
				<priority>1</priority>
			  </path-option>
			</path-options>
		  </primary>
		  <global>
			<config-items>
			  <source-address>{{.source}}</source-address>
			  <destination-address>{{.destination}}</destination-address>
			  <admin-state>enabled</admin-state>
			  <tunnel-name>{{.name}}</tunnel-name>
			</config-items>
		  </global>
		</tunnel>
	  </tunnels>
	</dn-rsvp:rsvp>
  </dn-protocol:protocols>
</dn-top:drivenets-top>
</config>
</edit-config>`
)

func MethodRsvpTunnelAdd(name string, source net.IP, destination net.IP) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(rsvpTunnelAddXml)).
		Execute(body, map[string]interface{}{
			"name":        name,
			"source":      source,
			"destination": destination,
		})
	return netconf.RawMethod(body.String())
}

func (h *DnHalImpl) AddTunnel(name string, source net.IP, destination net.IP, t TunnelType, haloAddr net.IP, haloNet net.IPNet) error {
	if t != RSVP {
		return fmt.Errorf("ERROR: Failed to delete tunnel %v. Reason: tunnel type %v is not supported", name, t)
	}

	session := NetConfConnector()
	reply, err := session.Exec(MethodRsvpTunnelAdd(name, source, destination))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	if ifc, err := GetTunnelInterface(name, t); err == nil {
		siIfc := SiInterface{
			Physical: ifc,
			Address:  haloAddr,
			Network:  haloNet,
		}
		if _, err := GetServiceInstanceInterfaces(SI_HALO_NAME); err != nil {
			log.Warnf("failed to get SI %s interfaces. Skip SI configuration", SI_HALO_NAME)
		} else {
			if err = AddSiInterface(SI_HALO_NAME, siIfc); err != nil {
				log.Warnf("failed to add SI interface: %s", err)
				return err
			}
			if err = AddInterfaceFlowMonitoring(ifc, FLOW_MONITORING_PROFILE, FLOW_MONITORING_TEMPLATE); err != nil {
				log.Warnf("failed to add interface %s flow monitoring: %s", ifc, err)
				return err
			}
		}
	}
	return commitChanges()
}

const (
	rsvpTunnelDeleteXml = `<edit-config>
<target><candidate/></target>
<default-operation>none</default-operation>
<error-option>rollback-on-error</error-option>
<config xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-protocol="http://drivenets.com/ns/yang/dn-protocol" xmlns:dn-rsvp="http://drivenets.com/ns/yang/dn-rsvp">
	<dn-top:drivenets-top>
		<dn-protocol:protocols>
			<dn-rsvp:rsvp>
			<tunnels>
				<tunnel operation="delete">
					<tunnel-name>{{.name}}</tunnel-name>
				</tunnel>
			</tunnels>
			</dn-rsvp:rsvp>
		</dn-protocol:protocols>
	</dn-top:drivenets-top>
</config>
</edit-config>`
)

func MethodRsvpTunnelDelete(name string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(rsvpTunnelDeleteXml)).
		Execute(body, map[string]interface{}{
			"name": name,
		})
	return netconf.RawMethod(body.String())
}

const SI_HALO_NAME = "halo"
const FLOW_MONITORING_TEMPLATE = "halo"
const FLOW_MONITORING_PROFILE = "halo"

func (h *DnHalImpl) DeleteTunnel(name string, t TunnelType) error {
	if t != RSVP {
		return fmt.Errorf("failed to delete tunnel %v. Reason: tunnel type %v is not supported", name, t)
	}

	session := NetConfConnector()
	reply, err := session.Exec(MethodRsvpTunnelDelete(name))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	} else {
		log.Infof("deleted tunnel %s, type %v", name, t)
	}
	if ifc, err := GetTunnelInterface(name, t); err == nil {
		if iflist, err := GetServiceInstanceInterfaces(SI_HALO_NAME); err != nil {
			log.Warnf("failed to get SI %s interfaces. Skip SI configuration", SI_HALO_NAME)
		} else {
			for _, haloIfc := range iflist {
				if ifc == haloIfc.Physical {
					log.Infof("found matching SI %s interface %s. Delete it from config", SI_HALO_NAME, ifc)
					if err = DeleteSiInterface(SI_HALO_NAME, ifc); err != nil {
						log.Warnf("failed to delete si interface: %s", err)
						return err
					}
					if err = DeleteInterfaceFlowMonitoring(ifc); err != nil {
						log.Warnf("failed to delete interface %s flow monitoring: %s ", ifc, err)
						return err
					}
				}
			}
		}
	}
	return commitChanges()
}

const (
	rsvpTunnelGetExplicitPath = `
<get-config>
    <source><running/></source>
	<filter type="subree">
		<drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
		<protocols>
		<rsvp>
			<explicit-paths>
			<explicit-path>
				<path-name>{{.name}}</path-name>
			</explicit-path>
			</explicit-paths>
		</rsvp>
		</protocols>
		</drivenets-top>
	</filter>
</get-config>`
)

func MethodRsvpTunnelGetExplicitPath(name string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(rsvpTunnelGetExplicitPath)).
		Execute(body, map[string]interface{}{
			"name": name,
		})
	return netconf.RawMethod(body.String())
}

func GetTunnelInterface(name string, t TunnelType) (string, error) {
	if t != RSVP {
		return "", fmt.Errorf("ERROR: Failed to delete tunnel %v. Reason: tunnel type %v is not supported", name, t)
	}

	session := NetConfConnector()
	path := fmt.Sprintf("%spath", name)
	reply, err := session.Exec(MethodRsvpTunnelGetExplicitPath(path))
	if err != nil {
		return "", fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	addr := net.ParseIP(regexp.MustCompile("<ipv4-address>([0-9.]+)</ipv4-address>").
		FindStringSubmatch(reply.Data)[1])
	ifc, err := GetInterfaceConnectedTo(addr)
	if err != nil {
		log.Info(err)
		return "", err
	}
	return ifc, nil
}

const (
	interfacesGetXml = `
<get-config>
    <source><running/></source>
	<filter type="subree">
		<drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
		<interfaces>
			<interface>
			<ipv4>
			<addresses>
				<address>
					<config-items/>
				</address>
			</addresses>
			</ipv4>
			</interface>
		</interfaces>
		</drivenets-top>
	</filter>
</get-config>`
)

type ipv4configXml struct {
	Ip           string `xml:"ip"`
	PrefixLength string `xml:"prefix-length"`
}

func MethodInterfacesGet() netconf.RawMethod {
	return netconf.RawMethod(interfacesGetXml)
}

func GetInterfaceConnectedTo(addr net.IP) (string, error) {
	session := NetConfConnector()
	reply, err := session.Exec(MethodInterfacesGet())
	if err != nil {
		return "", fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	d := xml.NewDecoder(strings.NewReader(reply.Data))
	var ifc string
	for {
		tok, err := d.Token()
		if tok == nil || err == io.EOF {
			break
		} else if err != nil {
			return "", fmt.Errorf("invalid token: %s. XML Response: %s", err, reply.Data)
		}
		switch ty := tok.(type) {
		case xml.StartElement:
			if ty.Name.Local == "name" {
				if tok, err := d.Token(); err == nil {
					if name, ok := tok.(xml.CharData); ok {
						ifc = string(name)
					}
				}
			}
			if ty.Name.Local == "config-items" {
				var cfg ipv4configXml
				if err = d.DecodeElement(&cfg, &ty); err != nil {
					return "", fmt.Errorf("invalid item: %s", err)
				}
				if _, ifnet, err := net.ParseCIDR(fmt.Sprintf("%s/%s", cfg.Ip, cfg.PrefixLength)); err == nil {
					if ifnet.Contains(addr) {
						return ifc, nil
					}
				}
			}
		default:
		}
	}
	return "", fmt.Errorf("no interface for IP %s", addr)
}

const serviceInstanceInterfacesGetXml = `
<get-config>
    <source><running/></source>
	<filter type="subree">
		<drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
		<service-instances>
		<instances>
			<instance>
				<si-name>{{.name}}</si-name>
			<config-items>
				<interfaces>
				<interface/>
				</interfaces>
			</config-items>
			</instance>
		</instances>
		</service-instances>
		</drivenets-top>
	</filter>
</get-config>`

func MethodServiceInstanceInterfacesGet(name string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(serviceInstanceInterfacesGetXml)).
		Execute(body, map[string]interface{}{
			"name": name,
		})
	return netconf.RawMethod(body.String())
}

type SiInterface struct {
	Physical string
	Address  net.IP
	Network  net.IPNet
}

type siInterfaceCfg struct {
	Name string `xml:"interface-name"`
	Addr string `xml:"ipv4-address"`
}

func GetServiceInstanceInterfaces(name string) ([]SiInterface, error) {
	session := NetConfConnector()
	reply, err := session.Exec(MethodServiceInstanceInterfacesGet(name))
	if err != nil {
		return nil, fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	d := xml.NewDecoder(strings.NewReader(reply.Data))
	ifc := make([]SiInterface, 0)
	for {
		tok, err := d.Token()
		if tok == nil || err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("invalid token: %s. XML Response: %s", err, reply.Data)
		}
		switch ty := tok.(type) {
		case xml.StartElement:
			if ty.Name.Local == "interface" {
				var cfg siInterfaceCfg
				if err = d.DecodeElement(&cfg, &ty); err != nil {
					return nil, fmt.Errorf("invalid item: %s", err)
				}
				if addr, network, err := net.ParseCIDR(cfg.Addr); err != nil {
					return nil, fmt.Errorf("invalid address: %s", cfg.Addr)
				} else {
					ifc = append(ifc, SiInterface{
						Physical: cfg.Name,
						Address:  addr,
						Network:  *network,
					})
				}
			}
		default:
		}
	}
	return ifc, nil
}

const siInterfaceDeleteXml = `
<edit-config>
  <target><candidate/></target>
  <default-operation>none</default-operation>
  <error-option>rollback-on-error</error-option>
  <config xmlns:dn-hyper-instances="http://drivenets.com/ns/yang/dn-hyper-instances" xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-hyper-service-instances="http://drivenets.com/ns/yang/dn-hyper-service-instances">
    <dn-top:drivenets-top>
      <dn-hyper-service-instances:service-instances>
        <dn-hyper-instances:instances>
          <instance>
			<si-name>{{.name}}</si-name>
            <config-items>
              <interfaces>
                <interface operation="delete">
                  <interface-name>{{.interface}}</interface-name>
                </interface>
              </interfaces>
            </config-items>
          </instance>
        </dn-hyper-instances:instances>
      </dn-hyper-service-instances:service-instances>
    </dn-top:drivenets-top>
  </config>
</edit-config>`

func MethodSiInterfaceDelete(name string, ifname string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(siInterfaceDeleteXml)).
		Execute(body, map[string]interface{}{
			"name":      name,
			"interface": ifname,
		})
	return netconf.RawMethod(body.String())
}

func DeleteSiInterface(name string, ifname string) error {
	session := NetConfConnector()
	reply, err := session.Exec(MethodSiInterfaceDelete(name, ifname))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	return nil
}

const siInterfaceAddXml = `
<edit-config>
  <target><candidate/></target>
  <default-operation>merge</default-operation>
  <error-option>rollback-on-error</error-option>
  <config xmlns:dn-hyper-instances="http://drivenets.com/ns/yang/dn-hyper-instances" xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-hyper-service-instances="http://drivenets.com/ns/yang/dn-hyper-service-instances">
    <dn-top:drivenets-top>
      <dn-hyper-service-instances:service-instances>
        <dn-hyper-instances:instances>
          <instance>
			<si-name>{{.name}}</si-name>
            <config-items>
              <interfaces>
                <interface>
                  <interface-name>{{.interface}}</interface-name>
				  <ipv4-address>{{.address}}</ipv4-address>
                </interface>
              </interfaces>
            </config-items>
          </instance>
        </dn-hyper-instances:instances>
      </dn-hyper-service-instances:service-instances>
    </dn-top:drivenets-top>
  </config>
</edit-config>`

func MethodSiInterfaceAdd(name string, ifname string, address string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(siInterfaceAddXml)).
		Execute(body, map[string]interface{}{
			"name":      name,
			"interface": ifname,
			"address":   address,
		})
	return netconf.RawMethod(body.String())
}

func AddSiInterface(name string, ifc SiInterface) error {
	session := NetConfConnector()
	plen, _ := ifc.Network.Mask.Size()
	reply, err := session.Exec(MethodSiInterfaceAdd(name, ifc.Physical,
		fmt.Sprintf("%s/%d", ifc.Address, plen)))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	return nil
}

var grpcConn *grpc.ClientConn
var mgmtClient pb.ManagementClient

const GRPC_MANAGEMENT_URL = "localhost:7732"

func GetMgmtClient() pb.ManagementClient {
	var err error
	if grpcConn == nil {
		ctx, _ := context.WithTimeout(context.Background(), time.Duration(1)*time.Second)
		grpcConn, err = grpc.DialContext(ctx, GRPC_MANAGEMENT_URL, grpc.WithInsecure())
		if err != nil {
			log.Errorf("Failed to connect to %s. Reason: %v", GRPC_MANAGEMENT_URL, err)
		}
		mgmtClient = pb.NewManagementClient(grpcConn)
	}
	return mgmtClient
}

const interfaceFlowMonitoringDeleteXml = `
<edit-config>
  <target><candidate/></target>
  <default-operation>none</default-operation>
  <error-option>rollback-on-error</error-option>
  <config xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-interfaces="http://drivenets.com/ns/yang/dn-interfaces" xmlns:dn-srv-flow-monitoring="http://drivenets.com/ns/yang/dn-srv-flow-monitoring">
  <dn-top:drivenets-top>
    <dn-interfaces:interfaces>
    <interface>
      <name>{{.interface}}</name>
      <dn-srv-flow-monitoring:flow-monitoring operation="delete"/>
    </interface>
    </dn-interfaces:interfaces>
  </dn-top:drivenets-top>
  </config>
</edit-config>`

func MethodInterfaceFlowMonitoringDelete(ifname string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(interfaceFlowMonitoringDeleteXml)).
		Execute(body, map[string]interface{}{
			"interface": ifname,
		})
	return netconf.RawMethod(body.String())
}

func DeleteInterfaceFlowMonitoring(ifname string) error {
	session := NetConfConnector()
	reply, err := session.Exec(MethodInterfaceFlowMonitoringDelete(ifname))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	return nil
}

const interfaceFlowMonitoringAddXml = `
<edit-config>
  <target><candidate/></target>
  <default-operation>merge</default-operation>
  <error-option>rollback-on-error</error-option>
  <config xmlns:dn-top="http://drivenets.com/ns/yang/dn-top" xmlns:dn-interfaces="http://drivenets.com/ns/yang/dn-interfaces" xmlns:dn-srv-flow-monitoring="http://drivenets.com/ns/yang/dn-srv-flow-monitoring">
    <dn-top:drivenets-top>
      <dn-interfaces:interfaces>
        <interface>
          <dn-srv-flow-monitoring:flow-monitoring>
            <config-items>
              <halo-template>
                <sampler-profile-name>{{.profile}}</sampler-profile-name>
                <template-name>{{.template}}</template-name>
                <direction>in</direction>
              </halo-template>
            </config-items>
          </dn-srv-flow-monitoring:flow-monitoring>
          <name>{{.interface}}</name>
        </interface>
      </dn-interfaces:interfaces>
    </dn-top:drivenets-top>
  </config>
</edit-config>`

func MethodInterfaceFlowMonitoringAdd(ifname string, profile string, tname string) netconf.RawMethod {
	body := &bytes.Buffer{}
	template.Must(template.New("").Parse(interfaceFlowMonitoringAddXml)).
		Execute(body, map[string]interface{}{
			"interface": ifname,
			"profile":   profile,
			"template":  tname,
		})
	return netconf.RawMethod(body.String())
}

func AddInterfaceFlowMonitoring(ifname string, profile string, tname string) error {
	session := NetConfConnector()
	reply, err := session.Exec(MethodInterfaceFlowMonitoringAdd(ifname, profile, tname))
	if err != nil {
		return fmt.Errorf("reply: %v, error: %s", reply, err)
	}
	return nil
}
