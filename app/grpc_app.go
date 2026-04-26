package app

import (
	"context"
	"errors"
	"io"

	pb "github.com/chinese-room-solutions/mass-sdk/app/rpc"
	gomodule "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// MaxGRPCMessageSize is the maximum gRPC message size for app communication.
// Raised from the default 4MB to support large payloads like base64-encoded images.
const MaxGRPCMessageSize = 64 * 1024 * 1024 // 64MB

// GRPCServer returns a gRPC server with increased max message size for app communication.
func GRPCServer(opts []grpc.ServerOption) *grpc.Server {
	opts = append(opts,
		grpc.MaxRecvMsgSize(MaxGRPCMessageSize),
		grpc.MaxSendMsgSize(MaxGRPCMessageSize),
	)
	return grpc.NewServer(opts...)
}

// AppGRPCPlugin implements gomodule.GRPCPlugin for the App service.
type AppGRPCPlugin struct {
	gomodule.NetRPCUnsupportedPlugin

	// Impl is set on the app side with the app's implementation.
	Impl AppInterface
}

var _ gomodule.GRPCPlugin = &AppGRPCPlugin{}

// GRPCServer is called on the app side.
func (p *AppGRPCPlugin) GRPCServer(_ *gomodule.GRPCBroker, s *grpc.Server) error {
	pb.RegisterAppServer(s, &appGRPCServer{impl: p.Impl})
	return nil
}

// GRPCClient is called on the host side.
func (p *AppGRPCPlugin) GRPCClient(
	_ context.Context,
	_ *gomodule.GRPCBroker,
	c *grpc.ClientConn,
) (any, error) {
	return &appGRPCClient{client: pb.NewAppClient(c)}, nil
}

// --- Host-side client adapter ---

type appGRPCClient struct {
	client pb.AppClient
}

var _ AppInterface = &appGRPCClient{}

func (c *appGRPCClient) GetInfo() (*AppInfo, error) {
	resp, err := c.client.GetInfo(context.Background(), &pb.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return protoToAppInfo(resp), nil
}

func (c *appGRPCClient) Health() (bool, error) {
	resp, err := c.client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		return false, err
	}
	return resp.Ok, nil
}

func (c *appGRPCClient) HandleRequest(
	ctx context.Context,
	method string,
	payload []byte,
) ([]byte, error) {
	resp, err := c.client.HandleRequest(ctx, &pb.HandleRequestRequest{
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Payload, nil
}

func (c *appGRPCClient) HandleRequestStream(
	ctx context.Context,
	method string,
	payload []byte,
	send func([]byte) error,
) error {
	stream, err := c.client.HandleRequestStream(ctx, &pb.HandleRequestRequest{
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return err
	}
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if chunk.Error != "" {
			return errors.New(chunk.Error)
		}
		if chunk.Done {
			return nil
		}
		if len(chunk.Payload) > 0 {
			if err := send(chunk.Payload); err != nil {
				return err
			}
		}
	}
}

func (c *appGRPCClient) SetLogLevel(level string) error {
	_, err := c.client.SetLogLevel(context.Background(), &pb.SetLogLevelRequest{Level: level})
	return err
}

// --- App-side server adapter ---

type appGRPCServer struct {
	pb.UnimplementedAppServer
	impl AppInterface
}

func (s *appGRPCServer) GetInfo(
	_ context.Context,
	_ *pb.GetInfoRequest,
) (*pb.AppInfo, error) {
	info, err := s.impl.GetInfo()
	if err != nil {
		return nil, err
	}
	return appInfoToProto(info), nil
}

func (s *appGRPCServer) Health(
	_ context.Context,
	_ *pb.HealthRequest,
) (*pb.HealthResponse, error) {
	ok, err := s.impl.Health()
	if err != nil {
		return nil, err
	}
	return &pb.HealthResponse{Ok: ok}, nil
}

func (s *appGRPCServer) HandleRequest(
	ctx context.Context,
	req *pb.HandleRequestRequest,
) (*pb.HandleRequestResponse, error) {
	payload, err := s.impl.HandleRequest(ctx, req.Method, req.Payload)
	if err != nil {
		return &pb.HandleRequestResponse{
			Error:     err.Error(),
			ErrorCode: "internal",
		}, nil
	}
	return &pb.HandleRequestResponse{Payload: payload}, nil
}

func (s *appGRPCServer) HandleRequestStream(
	req *pb.HandleRequestRequest,
	stream grpc.ServerStreamingServer[pb.HandleRequestStreamChunk],
) error {
	send := func(b []byte) error {
		return stream.Send(&pb.HandleRequestStreamChunk{Payload: b})
	}
	if err := s.impl.HandleRequestStream(stream.Context(), req.Method, req.Payload, send); err != nil {
		return stream.Send(&pb.HandleRequestStreamChunk{
			Error:     err.Error(),
			ErrorCode: "internal",
			Done:      true,
		})
	}
	return stream.Send(&pb.HandleRequestStreamChunk{Done: true})
}

func (s *appGRPCServer) SetLogLevel(
	_ context.Context,
	req *pb.SetLogLevelRequest,
) (*pb.SetLogLevelResponse, error) {
	if err := s.impl.SetLogLevel(req.Level); err != nil {
		return nil, err
	}
	return &pb.SetLogLevelResponse{}, nil
}
