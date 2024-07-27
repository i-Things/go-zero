package generate

import (
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
	"strconv"
	"strings"
)

var accessObj = Access{
	Access: map[string]*AccessInfo{},
}

const (
	accessCodePrefix = "accessCodePrefix"
	accessNamePrefix = "accessNamePrefix"
	accessGroup      = "accessGroup"
)

// （1(add)新增 2修改(modify) 3删除(delete) 4查询(find) 5其它(other)
func GetBusinessType(path string, or string) string {
	if or != "" {
		return or
	}
	paths := strings.Split(path, "/")
	opt := paths[len(paths)-1]
	switch opt {
	case "create", "multi-create":
		return "add"
	case "update", "multi-update":
		return "modify"
	case "delete", "multi-delete":
		return "delete"
	case "index", "read", "count":
		return "find"
	default:
		if strings.HasSuffix(opt, "send") {
			return "modify"
		}
		if strings.HasSuffix(opt, "read") || strings.HasSuffix(opt, "index") {
			return "find"
		}
		return "other"
	}
	return opt
}

func GetBusinessName(in string, or string) string {
	if in == "" {
		return or
	}
	switch in {
	case "add":
		return "新增"
	case "modify":
		return "修改"
	case "delete":
		return "删除"
	case "find":
		return "查询"
	default:
		return "操作"
	}
	return in
}

// FirstUpper 字符串首字母大写
func FirstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func Unquote(in string) string {
	ret, _ := strconv.Unquote(in)
	return ret
}
func BoolToInt(in string) int64 {
	if cast.ToBool(in) {
		return 1
	}
	return 2
}

func applyGenerate(p *plugin.Plugin, host string, basePath string) (*Access, error) {
	var routerMap = map[string][]ApiInfo{}
	for _, g := range p.Api.Service.Groups {
		properties := g.Annotation.Properties
		if _, ok := properties[accessGroup]; !ok {
			continue
		}
		acp := Unquote(properties[accessCodePrefix])
		a := AccessInfo{
			Group:      Unquote(properties[accessGroup]),
			IsNeedAuth: 1,
		}
		for _, r := range g.Routes {
			path := g.GetAnnotation("prefix") + r.Path
			if path[0] != '/' {
				path = "/" + path
			}
			bt := GetBusinessType(path, Unquote(r.AtDoc.Properties["businessType"]))
			ai := ApiInfo{
				AccessCode:   acp + FirstUpper(bt),
				Method:       strings.ToUpper(r.Method),
				Route:        path,
				Name:         Unquote(r.AtDoc.Properties["summary"]),
				BusinessType: bt,
				AuthType:     Unquote(r.AtDoc.Properties["authType"]),
				IsNeedAuth:   BoolToInt(Unquote(r.AtDoc.Properties["isNeedAuth"])),
			}
			if ai.Name == "" {
				ai.Name = Unquote(r.AtDoc.Text)
			}
			if ai.AuthType == "" {
				ai.AuthType = "all"
			}
			if ai.IsNeedAuth == 0 {
				ai.IsNeedAuth = 2
			}
			newA := a
			newA.Code = ai.AccessCode
			newA.IsNeedAuth = ai.IsNeedAuth
			newA.Name = GetBusinessName(bt, Unquote(r.AtDoc.Properties["businessName"])) + Unquote(g.GetAnnotation(accessNamePrefix))
			accessObj.Access[newA.Code] = &newA
			if _, ok := routerMap[ai.AccessCode]; !ok {
				routerMap[ai.AccessCode] = []ApiInfo{}
			}
			routerMap[ai.AccessCode] = append(routerMap[ai.AccessCode], ai)
		}
	}
	for code, r := range routerMap {
		a := accessObj.Access[code]
		a.Apis = r
	}
	return &accessObj, nil
}
