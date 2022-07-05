package main

import (
	"context"
	"github.com/go-programming-tour-book/tag-service/internal/middleware"
	pb "github.com/go-programming-tour-book/tag-service/proto"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
	"log"
)

func main() {
	ctx := context.Background()
	//clientConn, _ := GetClientConn(ctx, "localhost:8001", nil)
	clientConn, _ := GetClientConn(
		ctx,
		"localhost:8004",
		[]grpc.DialOption{
			grpc.WithBlock(),
			grpc.WithUnaryInterceptor(
				grpc_middleware.ChainUnaryClient(
					middleware.UnaryContextTimeout(),
				),
			),
		},
	)
	defer clientConn.Close()
	//初始化指定 RPC Proto Service 的客户端实例对象
	tagServiceClient := pb.NewTagServiceClient(clientConn)
	//发起指定 RPC 方法的调用
	resp, _ := tagServiceClient.GetTagList(ctx, &pb.GetTagListRequest{Name: "Golang"})

	log.Printf("resp: %v", resp)
}

func GetClientConn(ctx context.Context, target string, opts []grpc.DialOption) (*grpc.ClientConn, error) {
	//所要请求的服务端是非加密模式的，因此我们调用了 grpc.WithInsecure 方法禁用了此 ClientConn 的传输安全性验证
	opts = append(opts, grpc.WithInsecure())
	//创建给定目标的客户端连接
	return grpc.DialContext(ctx, target, opts...)
}
