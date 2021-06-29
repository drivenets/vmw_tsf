package hal

import (
	"context"

	pb "github.com/drivenets/vmw_tsf/pkg/hal/proto"
)

type StatsServer struct {
	pb.UnimplementedStatsServer
	hal *DnHalImpl
}

func NewStatsServer(h *DnHalImpl) *StatsServer {
	return &StatsServer{hal: h}
}

func (s *StatsServer) GetInterfaces(_ *pb.Empty, stream pb.Stats_GetInterfacesServer) error {
	for _, ifc := range copyLanInterfaces() {
		stream.Send(
			&pb.Interface{
				Name:    ifc.Upper,
				Speed:   ifc.Stats.Speed,
				RxBytes: ifc.Stats.RxBytes,
				RxBps:   ifc.Stats.RxBps,
				TxBytes: ifc.Stats.TxBytes,
				TxBps:   ifc.Stats.TxBps,
				Delay:   ifc.Stats.Link.Delay,
				Jitter:  ifc.Stats.Link.Jitter,
			})
	}
	for _, ifc := range copyWanInterfaces() {
		stream.Send(
			&pb.Interface{
				Name:    ifc.Upper,
				Speed:   ifc.Stats.Speed,
				RxBytes: ifc.Stats.RxBytes,
				RxBps:   ifc.Stats.RxBps,
				TxBytes: ifc.Stats.TxBytes,
				TxBps:   ifc.Stats.TxBps,
				Delay:   ifc.Stats.Link.Delay,
				Jitter:  ifc.Stats.Link.Jitter,
			})
	}
	return nil
}

func (s *StatsServer) GetAclCacheSize(_ context.Context, _ *pb.Empty) (*pb.CacheSize, error) {
	return &pb.CacheSize{Size: uint64(len(hal.aclRules))}, nil
}
