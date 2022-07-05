package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/go-programming-tour-book/tag-service/internal/middleware"
	pb "github.com/go-programming-tour-book/tag-service/proto"
	"github.com/go-programming-tour-book/tag-service/server"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"log"
	"net/http"
	"path"
	"strings"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/go-programming-tour-book/tag-service/pkg/swagger"
)

var port string

func init() {
	flag.StringVar(&port, "port", "8004", "启动端口号")
	flag.Parse()
}

//grpcHandlerFunc 关键方法 h2c.NewHandler 会返回一个 http.handler，其主要是在内部逻辑是拦截了所有 h2c 流量，
//然后根据不同的请求流量类型将其劫持并重定向到相应的 Hander 中去处理，最终以此达到同个端口上既提供 HTTP/1.1 又提供 HTTP/2 的功能了
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	//gRPC 服务的非加密模式的设置：关注代码中的"h2c"标识，“h2c” 标识允许通过明文 TCP 运行 HTTP/2 的协议，
	//此标识符用于 HTTP/1.1 升级标头字段以及标识 HTTP/2 over TCP，而官方标准库 golang.org/x/net/http2/h2c
	//实现了 HTTP/2 的未加密模式，我们直接使用即可
	return h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//gRPC 和 HTTP/1.1 的流量区分:
		// 1.对 ProtoMajor 进行判断，该字段代表客户端请求的版本号，客户端始终使用 HTTP/1.1 或 HTTP/2
		// 2.Header 头 Content-Type 的确定：grpc 的标志位 application/grpc 的确定
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	}), &http2.Server{})
}

func RunServer(port string) error {
	httpMux := runHttpServer()
	grpcS := runGrpcServer()
	gatewayMux := runGrpcGatewayServer()

	httpMux.Handle("/", gatewayMux)

	return http.ListenAndServe(":"+port, grpcHandlerFunc(grpcS, httpMux))
}

func runHttpServer() *http.ServeMux {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`pong`))
	})

	prefix := "/swagger-ui/"
	fileServer := http.FileServer(&assetfs.AssetFS{
		Asset:    swagger.Asset,
		AssetDir: swagger.AssetDir,
		Prefix:   "third_party/swagger-ui",
	})
	serveMux.Handle(prefix, http.StripPrefix(prefix, fileServer))
	serveMux.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "swagger.json") {
			http.NotFound(w, r)
			return
		}

		p := strings.TrimPrefix(r.URL.Path, "/swagger/")
		p = path.Join("proto", p)

		http.ServeFile(w, r, p)
	})
	return serveMux
}

func runGrpcServer() *grpc.Server {
	//grpc.ServerOption，gRPC Server 的相关属性都可以在此设置，例如：credentials、keepalive 等等参数
	opts := []grpc.ServerOption{
		/*//grpc.UnaryInterceptor(WorldInterceptor),
		//panic: The unary server interceptor was already set and may not be reset.
		grpc.UnaryInterceptor(HelloInterceptor),*/
		//grpc-ecosystem go-grpc-middleware提供的链式多拦截器
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			middleware.Recovery,
			middleware.ErrorLog,
			middleware.AccessLog,
		)),
	}
	s := grpc.NewServer(opts...)
	pb.RegisterTagServiceServer(s, server.NewTagServer())
	reflection.Register(s)

	return s
}

func runGrpcGatewayServer() *runtime.ServeMux {
	endpoint := "0.0.0.0:" + port
	//将grpcGateway错误转换为 RESTful API 的标准错误
	runtime.HTTPError = grpcGatewayError
	gwmux := runtime.NewServeMux()
	//在 grpc.DialOption 中通过设置 grpc.WithInsecure 指定了 Server 为非加密模式，否则程序在运行时将会出现问题，
	//因为 gRPC Server/Client 在启动和调用时，必须明确其是否加密
	dopts := []grpc.DialOption{grpc.WithInsecure()}
	//RegisterTagServiceHandlerFromEndpoint 方法去注册 TagServiceHandler 事件，其内部会自动转换并拨号到 gRPC Endpoint，
	//并在上下文结束后关闭连接
	_ = pb.RegisterTagServiceHandlerFromEndpoint(context.Background(), gwmux, endpoint, dopts)

	return gwmux
}

func HelloInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Println("你好")
	resp, err := handler(ctx, req)
	log.Println("再见")
	return resp, err
}

func WorldInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Println("Hello World")
	resp, err := handler(ctx, req)
	log.Println("GoodBye World")
	return resp, err
}

func main() {
	err := RunServer(port)
	if err != nil {
		log.Fatalf("Run Serve err: %v", err)
	}
}

type httpError struct {
	Code    int32  `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

//grpcGatewayError 针对所返回的 gRPC 错误进行了两次处理，将其转换为对应的 HTTP 状态码和对应的错误主体，
//以确保客户端能够根据 RESTful API 的标准来进行交互
func grpcGatewayError(ctx context.Context, _ *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
	s, ok := status.FromError(err)
	if !ok {
		s = status.New(codes.Unknown, err.Error())
	}

	httpError := httpError{Code: int32(s.Code()), Message: s.Message()}
	details := s.Details()
	for _, detail := range details {
		if v, ok := detail.(*pb.Error); ok {
			httpError.Code = v.Code
			httpError.Message = v.Message
		}
	}

	resp, _ := json.Marshal(httpError)
	w.Header().Set("Content-type", marshaler.ContentType())
	w.WriteHeader(runtime.HTTPStatusFromCode(s.Code()))
	_, _ = w.Write(resp)
}
