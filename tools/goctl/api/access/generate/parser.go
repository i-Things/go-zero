package generate

import (
	"fmt"
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
	"strconv"
	"strings"
	"unicode"
)

var accessObj = Access{
	Access: map[string]*AccessInfo{},
}

const (
	accessCodePrefix = "accessCodePrefix"
	accessNamePrefix = "accessNamePrefix"
	accessCode       = "accessCode"
	accessName       = "accessName"
	accessGroup      = "accessGroup"
	defaultNeedAuth  = "defaultNeedAuth"
	defaultAuthType  = "defaultAuthType"
)

// （1(add)新增 2修改(modify) 3删除(delete) 4查询(find) 5其它(other)
func GetBusinessType(path string, or string) string {
	if or != "" {
		return or
	}
	paths := strings.Split(path, "/")
	opt := paths[len(paths)-1]
	switch opt {
	case "create", "multi-create", "import":
		return "add"
	case "update", "multi-update":
		return "modify"
	case "delete", "multi-delete":
		return "delete"
	case "index", "read", "count", "tree":
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

// 下划线单词转为大写驼峰单词
func UderscoreToUpperCamelCase(s string) string {
	s = strings.Replace(s, "_", " ", -1)
	s = strings.Title(s)
	if s != "Id" {
		s = strings.ReplaceAll(s, "Id", "ID")
	}
	return strings.Replace(s, " ", "", -1)
}

// 下划线单词转为小写驼峰单词
func UderscoreToLowerCamelCase(s string) string {
	s = UderscoreToUpperCamelCase(s)
	return string(unicode.ToLower(rune(s[0]))) + s[1:]
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
		if acp == "" { //默认值是group的小驼峰格式
			group := properties["group"]
			group = strings.ReplaceAll(group, "/", "_")
			acp = UderscoreToLowerCamelCase(group)
		}
		AccessCode := Unquote(properties[accessCode])
		AccessName := Unquote(properties[accessName])
		authType := Unquote(properties[defaultAuthType])
		isNeedAuth := BoolToInt(Unquote(properties[defaultNeedAuth]))
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
				AuthType: func() string {
					at := Unquote(r.AtDoc.Properties["authType"])
					if at != "" {
						return at
					}
					return authType
				}(),
				IsNeedAuth: func() int64 {
					nt := Unquote(r.AtDoc.Properties["isNeedAuth"])
					if nt != "" {
						return BoolToInt(nt)
					}
					return isNeedAuth
				}(),
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
			newA.AuthType = ai.AuthType
			newA.Name = GetBusinessName(bt, Unquote(r.AtDoc.Properties["businessName"])) + Unquote(g.GetAnnotation(accessNamePrefix))
			if AccessName != "" {
				newA.Name = AccessName
			}
			if AccessCode != "" {
				newA.Code = AccessCode
				ai.AccessCode = AccessCode
			}
			if ac := Unquote(r.AtDoc.Properties[accessCode]); ac != "" {
				newA.Code = ac
				ai.AccessCode = ac
			}
			if ac := Unquote(r.AtDoc.Properties[accessName]); ac != "" {
				newA.Name = ac
			}
			oldAccess := accessObj.Access[newA.Code]
			if oldAccess != nil && oldAccess.IsNeedAuth != newA.IsNeedAuth {
				err := fmt.Errorf("授权的是否需要认证不统一,oldAccess:%#v \n oldApis:%#v\n newApi:%#v\n", oldAccess, routerMap[ai.AccessCode], ai)
				return nil, err
			}
			if oldAccess != nil && oldAccess.AuthType != newA.AuthType {
				err := fmt.Errorf("授权的授权类型不统一,oldAccess:%#v \n oldApis:%#v\n newApi:%#v\n", oldAccess, routerMap[ai.AccessCode], ai)
				return nil, err
			}
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
