package svc

import (
	{{.configImport}}
	"gitee.com/i-Things/share/ctxs"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config {{.config}}
	InitCtxsWare   rest.Middleware
	{{.middleware}}
}

func NewServiceContext(c {{.config}}) *ServiceContext {
	return &ServiceContext{
		Config: c,
		InitCtxsWare:   ctxs.InitMiddleware,
		{{.middlewareAssignment}}
	}
}
