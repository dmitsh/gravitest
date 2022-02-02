package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc"

	"github.com/dmitsh/gravitest/pkg/auth"
	"github.com/dmitsh/gravitest/proto"
)

const (
	CmdStart  string = "start"
	CmdStatus string = "status"
	CmdStream string = "stream"
	CmdStop   string = "stop"
)

var (
	clientCrtPath, clientKeyPath, caCrtPath string
)

func init() {
	flag.StringVar(&clientCrtPath, "cln.crt", "", "client certificate filepath")
	flag.StringVar(&clientKeyPath, "cln.key", "", "client key filepath")
	flag.StringVar(&caCrtPath, "ca.crt", "", "CA certificate path")
}

func main() {
	flag.Parse()

	cmd, args, err := validate()
	if err != nil {
		exit(err)
	}

	if err := runClient(cmd, args); err != nil {
		exit(err)
	}
}

func validate() (string, []string, error) {
	if len(clientCrtPath) == 0 {
		if clientCrtPath = os.Getenv("CLIENT_CERT"); len(clientCrtPath) == 0 {
			return "", nil, fmt.Errorf("missing client certificate")
		}
	}
	if len(clientKeyPath) == 0 {
		if clientKeyPath = os.Getenv("CLIENT_KEY"); len(clientKeyPath) == 0 {
			return "", nil, fmt.Errorf("missing client key")
		}
	}
	if len(caCrtPath) == 0 {
		if caCrtPath = os.Getenv("CA_CERT"); len(caCrtPath) == 0 {
			return "", nil, fmt.Errorf("missing CA certificate")
		}
	}
	var cmd string
	args := []string{}

	for _, arg := range flag.Args() {
		if len(cmd) == 0 {
			switch arg {
			case CmdStart, CmdStatus, CmdStream, CmdStop:
				cmd = arg
			default:
				return cmd, nil, fmt.Errorf("invalid command %v", arg)
			}
		} else {
			args = append(args, arg)
		}
	}
	if len(cmd) == 0 {
		return "", nil, fmt.Errorf("missing command")
	}
	if len(args) == 0 {
		return "", nil, fmt.Errorf("%q command requres arguments", cmd)
	}
	return cmd, args, nil
}

func exit(err error) {
	fmt.Printf("failed with error %v\n", err)
	os.Exit(1)
}

func runClient(cmd string, args []string) error {
	creds, err := auth.GetTLS(clientCrtPath, clientKeyPath, caCrtPath, false)
	if err != nil {
		return err
	}

	conn, err := grpc.Dial("localhost:12345", grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx := context.Background()
	client := proto.NewWorkerClient(conn)

	switch cmd {
	case CmdStart:
		resp, err := client.StartProcess(ctx, &proto.StartProcessRequest{Path: args[0], Args: args[1:]})
		if err != nil {
			return err
		}
		fmt.Println("Process UID:", resp.GetId())
	case CmdStop:
		_, err := client.StopProcess(ctx, &proto.JobId{Id: args[0]})
		if err != nil {
			return err
		}
		fmt.Println("Done")
	case CmdStatus:
		resp, err := client.GetProcessStatus(ctx, &proto.JobId{Id: args[0]})
		if err != nil {
			return err
		}
		procStatus := resp.GetProcStatus()
		fmt.Println("Process status:", procStatus)
		if procStatus == proto.Status_StatusStopped {
			fmt.Println("Exit status:", resp.GetExitStatus())
			if sig := resp.GetSignal(); sig != 0 {
				fmt.Println("Signal:", sig)
			}
		}
	case CmdStream:
		stream, err := client.StreamOutput(ctx, &proto.JobId{Id: args[0]})
		if err != nil {
			return err
		}
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			os.Stdout.Write(resp.GetData())
		}
	}
	return nil
}
