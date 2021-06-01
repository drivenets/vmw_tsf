package hal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Juniper/go-netconf/netconf"
	pb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"google.golang.org/grpc"
	//"log"
	"os"
	"strconv"
	"sync"
	"time"

	gfu "github.com/cloudflare/goflow/v3/utils"
	log "github.com/sirupsen/logrus"

	flowmessage "github.com/cloudflare/goflow/v3/pb"

	twamp "github.com/drivenets/vmw_tsf/tsf-twamp"
)

type FlowAggregate struct {
	start time.Time

	key   *FlowKey
	inIf  string
	outIf string

	packets uint64
	bytes   uint64
}

func (agg *FlowAggregate) ToTelemetry() *FlowTelemetry {
	millis := uint64(time.Since(agg.start).Milliseconds())
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

type DnHalImpl struct {
	mutex       sync.Mutex
	initialized bool
	grpcAddr    string
	interfaces  struct {
		twampAddr      map[string]string
		twampPort      map[string]int
		lower2upper    map[string]string
		upper2lower    map[string]string
		netflow2upper  map[uint32]string
		stats          map[string]*InterfaceTelemetry
		sampleInterval int
	}
	flows struct {
		state     *gfu.StateNetFlow
		aggregate map[string]*FlowAggregate
	}
}

var hal = &DnHalImpl{}

func NewDnHal() DnHal {
	if !hal.initialized {
		hal.Init()
	}
	return hal
}

const DRIVENETS_GRPC_ADDR = "localhost"

func (hal *DnHalImpl) InitFlows() {
	hal.flows.state = &gfu.StateNetFlow{
		Transport: hal,
		Logger:    log.StandardLogger(),
	}

	hal.flows.aggregate = make(map[string]*FlowAggregate)
}

func (hal *DnHalImpl) Init() {
	hal.mutex.Lock()
	defer hal.mutex.Unlock()

	var ok bool

	if hal.grpcAddr, ok = os.LookupEnv("GRPC_ADDR"); !ok {
		hal.grpcAddr = DRIVENETS_GRPC_ADDR
	}
	hal.grpcAddr = hal.grpcAddr + ":50051"

	hal.InitInterfaces()

	go monitorInterfaces()

	hal.InitFlows()

	go monitorFlows()

	hal.initialized = true
}

const DRIVENETS_INTERFACE_SAMPLE_INTERVAL = 15
const HALO_INTERFACES_COUNT = 2

func (hal *DnHalImpl) InitInterfaces() {
	var dnIf string
	var twamp string
	var twampPort string
	var haloIf string
	var ok bool

	hal.interfaces.twampAddr = make(map[string]string)
	hal.interfaces.twampPort = make(map[string]int)
	hal.interfaces.lower2upper = make(map[string]string)
	hal.interfaces.upper2lower = make(map[string]string)
	hal.interfaces.stats = make(map[string]*InterfaceTelemetry)
	hal.interfaces.netflow2upper = make(map[uint32]string)

	var interval string
	hal.interfaces.sampleInterval = DRIVENETS_INTERFACE_SAMPLE_INTERVAL
	if interval, ok = os.LookupEnv("IFC_SAMPLE"); ok {
		var err error
		if hal.interfaces.sampleInterval, err = strconv.Atoi(interval); err != nil {
			log.Fatalf("Failed to parse interface sampling interval: %s", interval)
		}
	}

	for idx := 0; idx < HALO_INTERFACES_COUNT; idx++ {
		haloIf = fmt.Sprintf("halo%d", idx)
		if dnIf, ok = os.LookupEnv(fmt.Sprintf("HALO%d_IFACE", idx)); !ok {
			dnIf = fmt.Sprintf("ge100-0/0/%d", idx)
		}
		if twamp, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP", idx)); !ok {
			twamp = "0.0.0.0:862"
			log.Error("Failed to get TWAMP server for: ", dnIf)
		}
		if twampPort, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP_PORT", idx)); !ok {
			twampPort = "10001"
			log.Error("Failed to get TWAMP server UDP ports for: ", dnIf)
		}
		hal.interfaces.twampAddr[haloIf] = twamp
		hal.interfaces.twampPort[haloIf], _ = strconv.Atoi(twampPort)
		hal.interfaces.lower2upper[dnIf] = haloIf
		hal.interfaces.upper2lower[haloIf] = dnIf
		hal.interfaces.stats[haloIf] = &InterfaceTelemetry{}
	}

	for haloIf, dnIf = range hal.interfaces.upper2lower {
		var ifIdx uint32
		var err error

		conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatal("failed to connect to gRPC server: %s. Reason: %w", hal.grpcAddr, err)
		}
		defer conn.Close()

		client := pb.NewGNMIClient(conn)

		if ifIdx, err = getDnIfIndex(client, dnIf); err != nil {
			log.Fatalf("Failed to get interface index for %s. Reason: %v", dnIf, err)
		}
		hal.interfaces.netflow2upper[ifIdx] = haloIf
		log.Printf("Interface: upper=%s, lower=%s, net-flow-index=%d",
			haloIf, dnIf, ifIdx)
	}
}

const DRIVENETS_IFINDEX_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/if-index"

func getDnIfIndex(client pb.GNMIClient, ifc string) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30*time.Minute))
	defer cancel()

	var pathList []*pb.Path
	idxPath := fmt.Sprintf(DRIVENETS_IFINDEX_PATH_TEMPLATE, ifc)
	pbPath, err := ygot.StringToPath(idxPath, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		return 0, fmt.Errorf("failed to convert %q to gNMI Path. Reason: %w", idxPath, err)
	}
	pathList = append(pathList, pbPath)
	resp, err := client.Get(ctx, &pb.GetRequest{
		Path:     pathList,
		Encoding: pb.Encoding_JSON,
	})
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
		log.Fatalf("failed to convert value: %q. Reason: %w", val, err)
	}
	return ifIndex, nil
}

const DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/counters/ethernet-counters"
const DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/interface-speed"

func subscribeForInterfaceStats(client pb.GNMIClient, ctx context.Context) (pb.GNMI_SubscribeClient, error) {
	sc, err := client.Subscribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to gNMI. Reason: %w", err)
	}

	for _, dnIf := range hal.interfaces.upper2lower {
		countersPath, _ := ygot.StringToPath(
			fmt.Sprintf(DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE, dnIf),
			ygot.StructuredPath, ygot.StringSlicePath)
		speedPath, _ := ygot.StringToPath(
			fmt.Sprintf(DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE, dnIf),
			ygot.StructuredPath, ygot.StringSlicePath)

		sc.Send(&pb.SubscribeRequest{
			Request: &pb.SubscribeRequest_Subscribe{
				Subscribe: &pb.SubscriptionList{
					Subscription: []*pb.Subscription{
						{
							Path:           speedPath,
							SampleInterval: uint64(time.Duration(15 * time.Second)),
						},
						{
							Path:           countersPath,
							SampleInterval: uint64(time.Duration(15 * time.Second)),
						},
					},
					Mode:     pb.SubscriptionList_STREAM,
					Encoding: pb.Encoding_JSON,
				},
			},
		})
	}
	return sc, nil
}

func monitorInterfaces() {
	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30*time.Minute))
	defer cancel()

	client := pb.NewGNMIClient(conn)
	sc, err := subscribeForInterfaceStats(client, ctx)

	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	twampTests := make(map[string]*twamp.TwampTest)
	for idx := 0; idx < HALO_INTERFACES_COUNT; idx++ {
		haloIf := fmt.Sprintf("halo%d", idx)
		twampAddr := hal.interfaces.twampAddr[haloIf]
		twampPort := hal.interfaces.twampPort[haloIf]

		twampClient := twamp.NewClient()
		twampConn, err := twampClient.Connect(twampAddr)
		if err != nil {
			log.Error(err)
			continue
		}
		defer twampConn.Close()

		twampSession, err := twampConn.CreateSession(
			twamp.TwampSessionConfig{
				SenderPort:   twampPort,
				ReceiverPort: twampPort + 1,
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
		twampTests[haloIf] = twampTest
	}

	for {
		response, err := sc.Recv()
		if err != nil {
			log.Fatalf("Failed to get response: %v", err)
		}

		for _, update := range response.GetUpdate().Update {
			var haloIf string
			for _, pEl := range update.Path.GetElem() {
				if pEl.Name == "interface" {
					haloIf = hal.interfaces.lower2upper[pEl.Key["name"]]
				}
			}
			//log.Println(string(update.Val.GetJsonVal()))
			//log.Println(proto.MarshalTextString(update.Val))
			//log.Println(update.Path.GetElem()[len(update.Path.GetElem())-1].Name)

			//log.Printf("Update content: %v\n", update.Val)
			lastPathElement := update.Path.GetElem()[len(update.Path.GetElem())-1].Name

			if lastPathElement == "interface-speed" {
				s, err := strconv.Atoi(string(update.Val.GetJsonVal()))
				if err != nil {
					log.Panic(err)
				}
				hal.interfaces.stats[haloIf].Speed = uint64(s)
				//log.Printf("Updated interface speed: %s\n", s)
			} else {
				ifc := hal.interfaces.stats[haloIf]
				err = json.Unmarshal(update.Val.GetJsonVal(), ifc)
				if err != nil {
					log.Fatalf("Failed to unmarshal: %s. Reason: %v",
						update.Val.GetJsonVal(), err)
				}
				//log.Printf("Updated interface counters: %v\n", *ifc)
			}

			if twampTest, ok := twampTests[haloIf]; ok {
				ifc := hal.interfaces.stats[haloIf]
				results := twampTest.RunX(5)
				ifc.Link.Delay = float64(results.Stat.Avg) / float64(time.Millisecond)
				ifc.Link.Jitter = float64(results.Stat.Jitter) / float64(time.Millisecond)
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
	for ifc, tl := range hal.interfaces.stats {
		err := v(ifc, tl)
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

		if inIf, ok = hal.interfaces.netflow2upper[msg.InIf]; !ok {
			inIf = "N/A"
		}
		if outIf, ok = hal.interfaces.netflow2upper[msg.OutIf]; !ok {
			outIf = "N/A"
		}

		// Update flows aggregate
		var agg *FlowAggregate
		key := fk.AsKey()
		aggregate := hal.flows.aggregate
		if agg, ok = aggregate[key]; ok {
			agg.bytes += msg.Bytes
			agg.packets += msg.Packets
		} else {
			agg = &FlowAggregate{
				start:   time.Now(),
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
	for _, agg := range aggregate {
		v(agg.key, agg.ToTelemetry())
	}
	return nil
}

func (*DnHalImpl) Steer(fk *FlowKey, nh string) error {
	var ok bool
	if nc.netconfUser, ok = os.LookupEnv("NETCONF_USER"); !ok {
		nc.netconfUser = "dnroot"
	}
	if nc.netconfPassword, ok = os.LookupEnv("NETCONF_PASSWORD"); !ok {
		nc.netconfPassword = "dnroot"
	}
	if nc.netconfHost, ok = os.LookupEnv("GRPC_ADDR"); !ok {
		nc.netconfHost = "localhost"
	}
	//log := log.StandardLogger()
	//log.SetLevel())
	//log := log.New(os.Stderr, "netconf ", 1)
	//netconf.SetLog(netconf.NewStdLog(log, netconf.LogDebug))
	session, err := netconf.DialSSH(
		nc.netconfHost,
		netconf.SSHConfigPassword(nc.netconfUser, nc.netconfPassword))
	if err != nil {
		return err
	}
	defer session.Close()

	log.Printf("Adding acl: %s:%d -> %s:%d", string(fk.SrcAddr), fk.SrcPort, string(fk.DstAddr), fk.DstPort)
	createAcl := fmt.Sprintf(AccessListConfig,
		accessListInitId,
		string(fk.SrcAddr),
		string(fk.DstAddr),
		string(fk.NextHop1),
		"any")
	_, err = session.Exec(netconf.RawMethod(createAcl))
	if err != nil {
		return err
	}

	log.Printf("Attaching rule %d to interface %s", accessListInitId, nh)
	attachAclToIface := fmt.Sprintf(InterfaceConfig, nh)
	_, err = session.Exec(netconf.RawMethod(attachAclToIface))
	if err != nil {
		return err
	}

	log.Printf("Committing changes")
	_, err = session.Exec(netconf.RawMethod(Commit))
	if err != nil {
		return err
	}

	accessListInitId += 10
	return nil
}
