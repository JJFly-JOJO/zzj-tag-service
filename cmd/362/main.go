package main

import (
	"flag"
	pb "github.com/go-programming-tour-book/tag-service/proto"
	"github.com/go-programming-tour-book/tag-service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
	"net/http"
)

//RPC服务在同一个端口适配两种协议 实现 gRPC（HTTP/2）和 HTTP/1.1 的支持
var grpcPort string
var httpPort string

func init() {
	flag.StringVar(&grpcPort, "grpc_port", "8001", "gRPC 启动端口号")
	flag.StringVar(&httpPort, "http_port", "9001", "HTTP 启动端口号")
	flag.Parse()
}

func RunHttpServer(port string) error {
	//初始化http多路复用器
	serveMux := http.NewServeMux()
	//新增了一个 /ping 路由及其 Handler
	serveMux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`pong`))
	})

	return http.ListenAndServe(":"+port, serveMux)
}

func RunGrpcServer(port string) error {
	s := grpc.NewServer()
	pb.RegisterTagServiceServer(s, server.NewTagServer())
	reflection.Register(s)
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	return s.Serve(lis)
}

func main() {
	//声明了一个 chan 用于接收 goroutine 的 err 信息
	errs := make(chan error)
	//由于监听 HTTP EndPoint 和 gRPC EndPoint 是一个阻塞的行为
	//因此分别在goroutine 中调用 RunHttpServer 和 RunGrpcServer 方法
	go func() {
		err := RunHttpServer(httpPort)
		if err != nil {
			errs <- err
		}
	}()
	go func() {
		err := RunGrpcServer(grpcPort)
		if err != nil {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		log.Fatalf("Run Server err: %v", err)
	}
}