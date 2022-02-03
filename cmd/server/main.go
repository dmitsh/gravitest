package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/dmitsh/gravitest/pkg/auth"
	"github.com/dmitsh/gravitest/pkg/engine"
	"github.com/dmitsh/gravitest/proto"
)

func main() {
	err := startServer()
	if err != nil {
		log.Printf("failed with error %v\n", err)
		os.Exit(1)
	}
}

func startServer() error {
	creds, err := auth.GetTLS("certs/server.crt", "certs/server.key", "certs/ca.crt", true)
	if err != nil {
		return err
	}
	server := grpc.NewServer(grpc.Creds(creds))

	worker := &WorkerServer{
		procManager: engine.NewProcManager(),
	}
	proto.RegisterWorkerServer(server, worker)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		l, err := net.Listen("tcp", ":12345")
		if err != nil {
			errCh <- err
		}
		log.Println("starting the server...")
		if err := server.Serve(l); err != nil {
			errCh <- err
		}
	}()

	select {
	case err = <-errCh:
		return err
	case <-stopCh:
		server.GracefulStop()
		log.Println("server stopped")
	}
	return nil
}

type WorkerServer struct {
	proto.UnimplementedWorkerServer

	procManager *engine.ProcManager
}

func (w *WorkerServer) StartProcess(ctx context.Context, req *proto.StartProcessRequest) (*proto.JobId, error) {
	// check that user command is not empty
	if len(req.GetPath()) == 0 {
		return &proto.JobId{}, errors.New("no command to run")
	}
	clientID := getClientID(ctx)
	log.Println("StartProcess: clientID:", clientID)
	uid, err := w.procManager.StartProcess(clientID, req.GetPath(), req.GetArgs()...)
	return &proto.JobId{Id: uid}, err
}

func (w *WorkerServer) StopProcess(ctx context.Context, req *proto.JobId) (*proto.Empty, error) {
	clientID := getClientID(ctx)
	log.Println("StopProcess: clientID:", clientID)
	err := w.procManager.StopProcess(clientID, req.GetId())
	return &proto.Empty{}, err
}

func (w *WorkerServer) GetProcessStatus(ctx context.Context, req *proto.JobId) (*proto.Status, error) {
	clientID := getClientID(ctx)
	log.Println("GetProcessStatus: clientID:", clientID)
	status, err := w.procManager.StatusProcess(clientID, req.GetId())
	return status, err
}

func (w *WorkerServer) StreamOutput(req *proto.JobId, srv proto.Worker_StreamOutputServer) error {
	ctx := srv.Context()
	clientID := getClientID(ctx)
	log.Println("StreamOutput: clientID:", clientID)
	reader, err := w.procManager.StreamOutput(clientID, req.GetId())
	if err != nil {
		return err
	}
	data := make([]byte, 512)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, err := reader.Read(data)
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if n > 0 {
				err = srv.Send(&proto.LogData{Data: data[:n]})
				if err != nil {
					return err
				}
			}
		}
	}
}

func getClientID(ctx context.Context) string {
	var clientID string
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			for _, item := range mtls.State.PeerCertificates {
				clientID = item.Subject.CommonName
			}
		}
	}
	return clientID
}
