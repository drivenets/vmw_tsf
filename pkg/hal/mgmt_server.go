package hal

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/drivenets/vmw_tsf/pkg/hal/proto"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type ManagementServer struct {
	pb.UnimplementedManagementServer
	hal *DnHalImpl
}

func NewManagementServer(h *DnHalImpl) *ManagementServer {
	return &ManagementServer{hal: h}
}

const DEFAULT_TWAMP_PORT = 862

func (s *ManagementServer) AddLanInterface(_ context.Context, args *pb.AddLanInterfaceArgs) (*pb.Empty, error) {
	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return &pb.Empty{}, fmt.Errorf("failed to connect to gRPC server: %s. Reason: %w", hal.grpcAddr, err)
	}
	defer conn.Close()
	client := gnmi.NewGNMIClient(conn)
	_, err = NewInterface(
		OptionInterfaceUpper(args.Upper),
		OptionInterfaceLower(args.Lower),
		OptionInterfaceUpdateNetFlowId(client),
	)
	if err != nil {
		log.Warnf("failed to add local interface %s. Reason: %v", args.Upper, err)
		return &pb.Empty{}, err
	}
	return &pb.Empty{}, nil
}

func parseTwampAddr(addr string) (string, string, error) {
	var peer string
	var port string
	tokens := strings.Split(addr, ":")
	switch len(tokens) {
	case 0:
		return "", "", fmt.Errorf("invalid twamp address: %s", addr)
	case 1:
		peer = tokens[0]
		port = fmt.Sprintf("%d", DEFAULT_TWAMP_PORT)
	default:
		peer = tokens[0]
		port = tokens[1]
	}
	return peer, port, nil
}

func (s *ManagementServer) AddWanInterface(_ context.Context, args *pb.AddWanInterfaceArgs) (*pb.Empty, error) {
	peer, port, err := parseTwampAddr(args.Twamp)
	if err != nil {
		return &pb.Empty{}, err
	}
	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return &pb.Empty{}, fmt.Errorf("failed to connect to gRPC server: %s. Reason: %w", hal.grpcAddr, err)
	}
	defer conn.Close()
	client := gnmi.NewGNMIClient(conn)
	_, err = NewInterface(
		OptionInterfaceUpper(args.Upper),
		OptionInterfaceLower(args.Lower),
		OptionInterfaceTwamp(peer, port),
		OptionInterfaceNextHop(args.NextHop),
		OptionInterfaceUpdateNetFlowId(client),
	)
	if err != nil {
		log.Warnf("failed to add wan interface %s. Reason: %v", args.Upper, err)
		return &pb.Empty{}, err
	}
	return &pb.Empty{}, nil
}

func (s *ManagementServer) DeleteInterface(_ context.Context, args *pb.DeleteInterfaceArgs) (*pb.Empty, error) {
	err := RemoveInterface(args.Upper)
	return &pb.Empty{}, err
}
