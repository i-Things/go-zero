package main

import (
    "context"
	"fmt"
	"gitee.com/unitedrhino/share/interceptors"

	{{.imports}}

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gitee.com/unitedrhino/share/utils"
)


func main() {
    defer utils.Recover(context.Background())
	var c config.Config
	utils.ConfMustLoad("etc/{{.serviceName}}.yaml", &c)
	svcCtx := svc.NewServiceContext(c)
    startup.Init(svcCtx)
	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
{{range .serviceNames}}       {{.Pkg}}.Register{{.GRPCService}}Server(grpcServer, {{.ServerPkg}}.New{{.Service}}Server(svcCtx))
{{end}}
		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()
	s.AddUnaryInterceptors(interceptors.Ctxs, interceptors.Error)

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
