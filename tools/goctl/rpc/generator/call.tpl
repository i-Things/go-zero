{{.head}}

package {{.filePackage}}

import (
	"context"

	{{.pbPackage}}
	{{.svcPackage}}
	{{if ne .pbPackage .protoGoPackage}}{{.protoGoPackage}}{{end}}

	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type (
	{{.alias}}

	{{.serviceName}} interface {
		{{.interface}}
	}

	default{{.serviceName}} struct {
		cli zrpc.Client
	}

	direct{{.serviceName}} struct {
		svcCtx *svc.ServiceContext
		svr    {{.protoPbPackage}}.{{.serviceName}}Server
	}
)

func New{{.serviceName}}(cli zrpc.Client) {{.serviceName}} {
	return &default{{.serviceName}}{
		cli: cli,
	}
}

func NewDirect{{.serviceName}}(svcCtx *svc.ServiceContext, svr {{.protoPbPackage}}.{{.serviceName}}Server) {{.serviceName}} {
	return &direct{{.serviceName}}{
		svr:    svr,
		svcCtx: svcCtx,
	}
}

{{.functions}}
