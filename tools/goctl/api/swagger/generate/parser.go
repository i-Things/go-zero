package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/zeromicro/go-zero/tools/goctl/api/spec"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
)

var strColon = []byte(":")

const (
	defaultOption   = "default"
	stringOption    = "string"
	optionalOption  = "optional"
	omitemptyOption = "omitempty"
	optionsOption   = "options"
	rangeOption     = "range"
	exampleOption   = "example"
	optionSeparator = "|"
	equalToken      = "="
	atRespDoc       = "@respdoc-"

	tagKeyHeader   = "header"
	tagKeyPath     = "path"
	tagKeyForm     = "form"
	tagKeyJson     = "json"
	tagKeyString   = "string"
	tagKeyValidate = "validate"
)

func parseRangeOption(option string) (float64, float64, bool) {
	const str = "\\[([+-]?\\d+(\\.\\d+)?):([+-]?\\d+(\\.\\d+)?)\\]"
	result := regexp.MustCompile(str).FindStringSubmatch(option)
	if len(result) != 5 {
		return 0, 0, false
	}

	min, err := strconv.ParseFloat(result[1], 64)
	if err != nil {
		return 0, 0, false
	}

	max, err := strconv.ParseFloat(result[3], 64)
	if err != nil {
		return 0, 0, false
	}

	if max < min {
		return min, min, true
	}
	return min, max, true
}

func applyGenerate(p *plugin.Plugin, host string, basePath string, schemes string) (*swaggerObject, error) {
	title, _ := strconv.Unquote(p.Api.Info.Properties["title"])
	version, _ := strconv.Unquote(p.Api.Info.Properties["version"])
	desc, _ := strconv.Unquote(p.Api.Info.Properties["desc"])

	s := swaggerObject{
		Swagger:           "2.0",
		Schemes:           []string{"http", "https"},
		Consumes:          []string{"application/json"},
		Produces:          []string{"application/json"},
		Paths:             make(swaggerPathsObject),
		Definitions:       make(swaggerDefinitionsObject),
		StreamDefinitions: make(swaggerDefinitionsObject),
		Info: swaggerInfoObject{
			Title:       title,
			Version:     version,
			Description: desc,
		},
	}
	if len(host) > 0 {
		s.Host = host
	}
	if len(basePath) > 0 {
		s.BasePath = basePath
	}

	if len(schemes) > 0 {
		supportedSchemes := []string{"http", "https", "ws", "wss"}
		ss := strings.Split(schemes, ",")
		for i := range ss {
			scheme := ss[i]
			scheme = strings.TrimSpace(scheme)
			if !contains(supportedSchemes, scheme) {
				log.Fatalf("unsupport scheme: [%s], only support [http, https, ws, wss]", scheme)
			}
			ss[i] = scheme
		}
		s.Schemes = ss
	}
	s.SecurityDefinitions = swaggerSecurityDefinitionsObject{}
	newSecDefValue := swaggerSecuritySchemeObject{}
	newSecDefValue.Name = "Authorization"
	newSecDefValue.Description = "Enter JWT Bearer token **_only_**"
	newSecDefValue.Type = "apiKey"
	newSecDefValue.In = "header"
	s.SecurityDefinitions["apiKey"] = newSecDefValue

	// s.Security = append(s.Security, swaggerSecurityRequirementObject{"apiKey": []string{}})

	requestResponseRefs := refMap{}
	renderServiceRoutes(p.Api.Service, p.Api.Service.Groups, s.Paths, requestResponseRefs)
	m := messageMap{}

	renderReplyAsDefinition(s.Definitions, m, p.Api.Types, requestResponseRefs)

	return &s, nil
}

// FirstUpper 字符串首字母大写
func FirstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func GenStr(in string) string {
	ps := strings.Split(in, "/")
	var newPs []string
	for _, v := range ps {
		if v == "" {
			continue
		}
		newPs = append(newPs, FirstUpper(v))
	}
	return strings.Join(newPs, "")
}
func GenLowStr(in string) string {
	s := GenStr(in)
	return strings.ToLower(s[:1]) + s[1:]
}

func renderServiceRoutes(service spec.Service, groups []spec.Group, paths swaggerPathsObject, requestResponseRefs refMap) {
	for _, group := range groups {
		var prefix string
		if p, ok := group.Annotation.Properties["prefix"]; ok {
			//有前缀
			prefix = GenStr(p)
		}
		for _, route := range group.Routes {
			var (
				pathParamMap             = make(map[string]swaggerParameterObject)
				parameters               swaggerParametersObject
				hasBody                  bool
				hasFile                  bool
				containForm, containJson bool
			)

			path := group.GetAnnotation("prefix") + route.Path
			if path[0] != '/' {
				path = "/" + path
			}

			if m := strings.ToUpper(route.Method); m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch {
				hasBody = true
			}

			if countParams(path) > 0 {
				p := strings.Split(path, "/")
				for i := range p {
					part := p[i]
					if strings.Contains(part, ":") {
						key := strings.TrimPrefix(p[i], ":")
						path = strings.Replace(path, fmt.Sprintf(":%s", key), fmt.Sprintf("{%s}", key), 1)
						spo := swaggerParameterObject{
							Name:     key,
							In:       "path",
							Required: true,
							Type:     "string",
						}

						// extend the comment functionality
						// to allow query string parameters definitions
						// EXAMPLE:
						// @doc(
						// 	summary: "Get Cart"
						// 	description: "returns a shopping cart if one exists"
						// 	customerId: "customer id"
						// )
						//
						// the format for a parameter is
						// paramName: "the param description"
						//

						prop := route.AtDoc.Properties[key]
						if prop != "" {
							// remove quotes
							spo.Description = strings.Trim(prop, "\"")
						}

						pathParamMap[spo.Name] = spo
					}
				}
			}
			if defineStruct, ok := route.RequestType.(spec.DefineStruct); ok {
				for _, member := range defineStruct.Members {
					f, j := renderMember(pathParamMap, &parameters, member, hasBody)
					if f {
						containForm = true
					}
					if j {
						containJson = true
					}
				}

				if len(pathParamMap) > 0 {
					for _, p := range pathParamMap {
						parameters = append(parameters, p)
					}
				}
				if hasBody {
					if len(route.RequestType.Name()) > 0 {
						// If there is a file key in the @doc, add the file parameter, and the parameter will be changed to formData
						// Example:
						// @doc(
						// 	injectFormdataParam: "file"
						// )
						if fileKey, ok := route.AtDoc.Properties["injectFormdataParam"]; ok {
							hasFile = true
							// First, add a file parameter to the form data
							fileParameter := swaggerParameterObject{
								Name:     strings.Trim(fileKey, "\""),
								Type:     "file",
								In:       "formData",
								Required: true,
							}

							parameters = append(parameters, fileParameter)

							// Construct the remaining parameters
							for _, member := range defineStruct.Members {
								required := true
								// Whether the parameter is mandatory
								for _, tag := range member.Tags() {
									for _, option := range tag.Options {
										if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
											required = false
										}
									}
								}

								// Obtain the parameter type
								tName := member.Type.Name()
								if strings.Contains(member.Tag, ",string") {
									tName = "string"
								}
								tempKind := swaggerMapTypes[strings.Replace(tName, "[]", "", -1)]
								ftype, format, ok := primitiveSchema(tempKind, tName)
								if !ok {
									ftype = tempKind.String()
									format = "UNKNOWN"
								}

								// Construction parameters
								fileParameter := swaggerParameterObject{
									Name:        strings.ToLower(strings.Trim(member.Name, "\"")),
									Type:        ftype,
									Format:      format,
									In:          "formData",
									Required:    required,
									Description: member.GetComment(),
								}

								parameters = append(parameters, fileParameter)
							}
						} else {
							reqRef := fmt.Sprintf("#/definitions/%s", route.RequestType.Name())

							schema := swaggerSchemaObject{
								schemaCore: schemaCore{
									Ref: reqRef,
								},
							}

							parameter := swaggerParameterObject{
								Name:     "body",
								In:       "body",
								Required: true,
								Schema:   &schema,
							}

							doc := strings.Join(route.RequestType.Documents(), ",")
							doc = strings.Replace(doc, "//", "", -1)

							if doc != "" {
								parameter.Description = doc
							}

							parameters = append(parameters, parameter)
						}
					}
				}
			}

			pathItemObject, ok := paths[path]
			if !ok {
				pathItemObject = swaggerPathItemObject{}
			}

			desc := "A successful response."
			respSchema := swaggerSchemaObject{
				schemaCore: schemaCore{Type: "object"},
				Properties: &swaggerSchemaObjectProperties{
					{Key: "code",
						Value: &swaggerSchemaObject{
							schemaCore: schemaCore{Type: "integer", Default: "200"},
							Title:      "返回code",
						},
					},
					{Key: "msg",
						Value: &swaggerSchemaObject{
							schemaCore: schemaCore{Type: "string"},
							Title:      "返回的消息",
						},
					},
				},
			}
			// respRef := swaggerSchemaObject{}
			if route.ResponseType != nil && len(route.ResponseType.Name()) > 0 {
				if strings.HasPrefix(route.ResponseType.Name(), "[]") {

					refTypeName := strings.Replace(route.ResponseType.Name(), "[", "", 1)
					refTypeName = strings.Replace(refTypeName, "]", "", 1)

					respSchema.Type = "array"
					respSchema.Items = &swaggerItemsObject{Ref: fmt.Sprintf("#/definitions/%s", refTypeName)}
				} else {
					*respSchema.Properties = append(*respSchema.Properties, keyVal{
						Key: "data",
						Value: &swaggerSchemaObject{
							schemaCore: schemaCore{Ref: fmt.Sprintf("#/definitions/%s", route.ResponseType.Name())},
						},
					})
				}
			}
			tags := []string{service.Name}
			if value := group.GetAnnotation("group"); len(value) > 0 {
				tags = []string{value, GenLowStr(value)}
			}

			if value := group.GetAnnotation("swtags"); len(value) > 0 {
				tags = []string{value}
			}

			operationObject := &swaggerOperationObject{
				Tags:       tags,
				Parameters: parameters,
				Responses: swaggerResponsesObject{
					"200": swaggerResponseObject{
						Description: desc,
						Schema:      respSchema,
					},
				},
			}
			// if request has body, there is no way to distinguish query param and form param.
			// because they both share the "form" tag, the same param will appear in both query and body.
			if hasBody && containForm && !containJson {
				operationObject.Consumes = []string{"multipart/form-data"}
				if !hasFile {
					operationObject.Consumes = []string{"application/x-www-form-urlencoded"}
				}
			}

			for _, v := range route.Doc {
				markerIndex := strings.Index(v, atRespDoc)
				if markerIndex >= 0 {
					l := strings.Index(v, "(")
					r := strings.Index(v, ")")
					code := strings.TrimSpace(v[markerIndex+len(atRespDoc) : l])
					var comment string
					commentIndex := strings.Index(v, "//")
					if commentIndex > 0 {
						comment = strings.TrimSpace(strings.Trim(v[commentIndex+2:], "*/"))
					}
					content := strings.TrimSpace(v[l+1 : r])
					if strings.Index(v, ":") > 0 {
						lines := strings.Split(content, "\n")
						kv := make(map[string]string, len(lines))
						for _, line := range lines {
							sep := strings.Index(line, ":")
							key := strings.TrimSpace(line[:sep])
							value := strings.TrimSpace(line[sep+1:])
							kv[key] = value
						}
						kvByte, err := json.Marshal(kv)
						if err != nil {
							continue
						}
						operationObject.Responses[code] = swaggerResponseObject{
							Description: comment,
							Schema: swaggerSchemaObject{
								schemaCore: schemaCore{
									Example: string(kvByte),
								},
							},
						}
					} else if len(content) > 0 {
						operationObject.Responses[code] = swaggerResponseObject{
							Description: comment,
							Schema: swaggerSchemaObject{
								schemaCore: schemaCore{
									Ref: fmt.Sprintf("#/definitions/%s", content),
								},
							},
						}
					}
				}
			}

			// set OperationID

			operationObject.OperationID = fmt.Sprintf("%s%s%s", route.Method, prefix, GenStr(route.Path))

			for _, param := range operationObject.Parameters {
				if param.Schema != nil && param.Schema.Ref != "" {
					requestResponseRefs[param.Schema.Ref] = struct{}{}
				}
			}
			operationObject.Summary = strings.ReplaceAll(route.JoinedDoc(), "\"", "")

			if len(route.AtDoc.Properties) > 0 {
				operationObject.Description, _ = strconv.Unquote(route.AtDoc.Properties["description"])
			}

			operationObject.Description = strings.ReplaceAll(operationObject.Description, "\"", "")

			if group.Annotation.Properties["jwt"] != "" {
				operationObject.Security = &[]swaggerSecurityRequirementObject{{"apiKey": []string{}}}
			} else if group.Annotation.Properties["security"] == "true" {
				operationObject.Security = &[]swaggerSecurityRequirementObject{{"apiKey": []string{}}}
			}

			switch strings.ToUpper(route.Method) {
			case http.MethodGet:
				pathItemObject.Get = operationObject
			case http.MethodPost:
				pathItemObject.Post = operationObject
			case http.MethodDelete:
				pathItemObject.Delete = operationObject
			case http.MethodPut:
				pathItemObject.Put = operationObject
			case http.MethodPatch:
				pathItemObject.Patch = operationObject
			}

			paths[path] = pathItemObject
		}
	}
}

// renderMember collect param property from spec.Member, return whether there exists form fields and json fields.
func renderMember(pathParamMap map[string]swaggerParameterObject,
	parameters *swaggerParametersObject, member spec.Member, hasBody bool) (containForm, containJson bool) {
	if embedStruct, isEmbed := member.Type.(spec.DefineStruct); isEmbed {
		for _, m := range embedStruct.Members {
			f, j := renderMember(pathParamMap, parameters, m, hasBody)
			if f {
				containForm = true
			}
			if j {
				containJson = true
			}
		}
		return
	}

	p := renderStruct(member)

	if hasBody && p.In == "" {
		return
	}
	if p.In == "body" {
		containJson = true
		return
	}
	if p.In == "query" {
		containForm = true
	}
	// default in query?
	if p.In == "" {
		p.In = "query"
	}

	// overwrite path parameter if we get a user defined one from struct.
	if op, ok := pathParamMap[p.Name]; p.In == "path" && ok {
		if p.Description == "" && op.Description != "" {
			p.Description = op.Description
		}
		delete(pathParamMap, p.Name)
	}
	*parameters = append(*parameters, p)
	return
}

func fillValidateOption(s *swaggerSchemaObject, opt string) {
	kv := strings.SplitN(opt, "=", 2)
	if len(kv) != 2 {
		return
	}
	switch kv[0] {
	case "oneof":
		var es []string
		// oneof='red green' 'blue yellow'
		if strings.Contains(kv[1], "'") {
			es = strings.Split(kv[1], "' '")
			es[0] = strings.TrimPrefix(es[0], "'")
			es[len(es)-1] = strings.TrimSuffix(es[len(es)-1], "'")
		} else {
			es = strings.Split(kv[1], " ")
		}
		s.Enum = es
	case "min", "gte", "gt":
		switch s.Type {
		case "number", "integer":
			s.Minimum, _ = strconv.ParseFloat(kv[1], 64)
		case "array", "string":
			v, err := strconv.ParseUint(kv[1], 10, 64)
			if err != nil {
				break
			}
			if s.Type == "array" {
				s.MinItems = v
			} else {
				s.MinLength = v
			}
		}
		if kv[0] == "gt" {
			s.ExclusiveMinimum = true
		}
	case "max", "lte", "lt":
		switch s.Type {
		case "number", "integer":
			s.Maximum, _ = strconv.ParseFloat(kv[1], 64)
		case "array", "string":
			v, err := strconv.ParseUint(kv[1], 10, 64)
			if err != nil {
				break
			}
			if s.Type == "array" {
				s.MaxItems = v
			} else {
				s.MaxLength = v
			}
		}
		if kv[0] == "lt" {
			s.ExclusiveMaximum = true
		}
	}
}
func fillValidate(s *swaggerSchemaObject, tag *spec.Tag) {
	if tag.Key != tagKeyValidate {
		return
	}
	fillValidateOption(s, tag.Name)
	for _, opt := range tag.Options {
		fillValidateOption(s, opt)
	}
}

// renderStruct only need to deal with params in header/path/query
func renderStruct(member spec.Member) swaggerParameterObject {
	tempKind := swaggerMapTypes[strings.Replace(member.Type.Name(), "[]", "", -1)]

	ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())
	if !ok {
		ftype = tempKind.String()
		format = "UNKNOWN"
	}
	sp := swaggerParameterObject{In: "", Type: ftype, Format: format, Schema: new(swaggerSchemaObject)}

	for _, tag := range member.Tags() {
		switch tag.Key {
		case tagKeyHeader:
			sp.In = "header"
		case tagKeyPath:
			sp.In = "path"
		case tagKeyForm:
			sp.In = "query"
		case tagKeyJson:
			sp.In = "body"
		case tagKeyValidate:
			fillValidate(sp.Schema, tag)
			sp.Enum = sp.Schema.Enum
			continue
		default:
			continue
		}

		sp.Name = tag.Name
		if len(tag.Options) == 0 {
			sp.Required = true
			continue
		}

		required := true
		for _, option := range tag.Options {
			if strings.HasPrefix(option, optionsOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					sp.Enum = strings.Split(segs[1], optionSeparator)
				}
			}

			if strings.HasPrefix(option, rangeOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						sp.Schema.Minimum = min
						sp.Schema.Maximum = max
					}
				}
			}

			if strings.HasPrefix(option, defaultOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Default = segs[1]
				}
			} else if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
				required = false
			}

			if strings.HasPrefix(option, exampleOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Example = segs[1]
				}
			}
		}
		sp.Required = required
	}

	if len(member.Comment) > 0 {
		sp.Description = strings.Replace(strings.TrimLeft(member.Comment, "//"), "\\n", "\n", -1)
	}

	return sp
}

func renderReplyAsDefinition(d swaggerDefinitionsObject, m messageMap, p []spec.Type, refs refMap) {
	for _, i2 := range p {
		var formFields, untaggedFields swaggerSchemaObjectProperties

		schema := swaggerSchemaObject{
			schemaCore: schemaCore{
				Type: "object",
			},
			Properties: new(swaggerSchemaObjectProperties),
		}
		defineStruct, _ := i2.(spec.DefineStruct)

		schema.Title = defineStruct.Name()

		for i, member := range defineStruct.Members {
			for _, tag := range member.Tags() {
				if tag.Key != tagKeyForm && tag.Key != tagKeyJson {
					continue
				}
				if len(tag.Options) == 0 {
					if !contains(schema.Required, tag.Name) && tag.Name != "required" {
						schema.Required = append(schema.Required, tag.Name)
					}
					continue
				}

				required := true
				for _, option := range tag.Options {
					// case strings.HasPrefix(option, defaultOption):
					// case strings.HasPrefix(option, optionsOption):

					if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
						required = false
					}
					if option == tagKeyString {
						member.Type = toString(member.Type)
						defineStruct.Members[i] = member
					}
				}

				if required && !contains(schema.Required, tag.Name) {
					schema.Required = append(schema.Required, tag.Name)
				}
			}
			collectProperties(schema.Properties, &formFields, &untaggedFields, member)

		}
		// if there exists any json fields, form fields are ignored (considered to be params in query).
		if len(*schema.Properties) == 0 && len(formFields) > 0 {
			*schema.Properties = formFields
		}
		if len(untaggedFields) > 0 {
			*schema.Properties = append(*schema.Properties, untaggedFields...)
		}

		d[i2.Name()] = schema
	}
}

var set = map[string]struct{}{}

func toString(t spec.Type) spec.Type {
	if _, ok := set[t.Name()]; !ok {
		fmt.Println("转换成string类型的有:" + t.Name())
		set[t.Name()] = struct{}{}
	}
	switch t.(type) {
	case spec.PrimitiveType:
		pt := t.(spec.PrimitiveType)
		pt.RawName = "string"
		t = pt
		return t
	case spec.PointerType:
		t = spec.PointerType{RawName: "*string", Type: spec.PrimitiveType{RawName: "string"}}
		return t
	case spec.ArrayType:
		pt := t.(spec.ArrayType)
		t = spec.ArrayType{RawName: "[]string", Value: toString(pt.Value)}
		return t
	default:
		fmt.Println("不支持")
	}
	return t
}

func collectProperties(jsonFields, formFields, untaggedFields *swaggerSchemaObjectProperties, member spec.Member) {
	in := fieldIn(member)
	if in == tagKeyHeader || in == tagKeyPath {
		return
	}

	name := member.Name
	if tag, err := member.GetPropertyName(); err == nil {
		name = tag
	}
	if name == "" {
		memberStruct, _ := member.Type.(spec.DefineStruct)
		// currently go-zero does not support show members of nested struct over 2 levels(include).
		// but openapi 2.0 does not support inline schema, we have no choice but use an empty properties name
		// which is not friendly to the user.
		if len(memberStruct.Members) > 0 {
			for _, m := range memberStruct.Members {
				collectProperties(jsonFields, formFields, untaggedFields, m)
			}
			return
		}
	}

	kv := keyVal{Key: name, Value: schemaOfField(member)}
	switch in {
	case tagKeyJson:
		*jsonFields = append(*jsonFields, kv)
	case tagKeyForm:
		*formFields = append(*formFields, kv)
	default:
		*untaggedFields = append(*untaggedFields, kv)
	}
}

func fieldIn(member spec.Member) string {
	for _, tag := range member.Tags() {
		if tag.Key == tagKeyPath || tag.Key == tagKeyHeader || tag.Key == tagKeyForm || tag.Key == tagKeyJson {
			return tag.Key
		}
	}

	return ""
}

func schemaOfField(member spec.Member) swaggerSchemaObject {
	ret := swaggerSchemaObject{}

	var core schemaCore
	t := member.Type.Name()
	kind := swaggerMapTypes[t]
	_, isMap := member.Type.(spec.MapType)
	if isMap {
		kind = reflect.Map
	}
	var props *swaggerSchemaObjectProperties

	comment := member.GetComment()
	comment = strings.Replace(strings.Replace(comment, "//", "", -1), "\\n", "\n", -1)

	switch ft := kind; ft {
	case reflect.Invalid: //[]Struct 也有可能是 Struct
		// []Struct
		// map[ArrayType:map[Star:map[StringExpr:UserSearchReq] StringExpr:*UserSearchReq] StringExpr:[]*UserSearchReq]
		refTypeName := strings.Replace(member.Type.Name(), "[", "", 1)
		refTypeName = strings.Replace(refTypeName, "]", "", 1)
		refTypeName = strings.Replace(refTypeName, "*", "", 1)
		refTypeName = strings.Replace(refTypeName, "{", "", 1)
		refTypeName = strings.Replace(refTypeName, "}", "", 1)
		// interface

		if refTypeName == "interface" {
			core = schemaCore{Type: "object"}
		} else if refTypeName == "mapstringstring" {
			core = schemaCore{Type: "object"}
		} else if strings.HasPrefix(refTypeName, "[]") {
			core = schemaCore{Type: "array"}

			tempKind := swaggerMapTypes[strings.Replace(refTypeName, "[]", "", -1)]
			ftype, format, ok := primitiveSchema(tempKind, refTypeName)
			if ok {
				core.Items = &swaggerItemsObject{Type: ftype, Format: format}
			} else {
				core.Items = &swaggerItemsObject{Type: ft.String(), Format: "UNKNOWN"}
			}

		} else {
			core = schemaCore{
				Ref: "#/definitions/" + refTypeName,
			}
		}
	case reflect.Slice:
		tempKind := swaggerMapTypes[strings.Replace(member.Type.Name(), "[]", "", -1)]
		ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())

		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	default:
		ftype, format, ok := primitiveSchema(ft, member.Type.Name())
		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	}

	switch ft := kind; ft {
	case reflect.Slice:
		ret = swaggerSchemaObject{
			schemaCore: schemaCore{
				Type:  "array",
				Items: (*swaggerItemsObject)(&core),
			},
		}
	case reflect.Invalid:
		// 判断是否数组
		if strings.HasPrefix(member.Type.Name(), "[]") {
			ret = swaggerSchemaObject{
				schemaCore: schemaCore{
					Type:  "array",
					Items: (*swaggerItemsObject)(&core),
				},
			}
		} else {
			ret = swaggerSchemaObject{
				schemaCore: core,
				Properties: props,
			}
		}
		if strings.HasPrefix(member.Type.Name(), "map") {
			fmt.Println("暂不支持map类型")
		}
	default:
		ret = swaggerSchemaObject{
			schemaCore: core,
			Properties: props,
		}
	}
	ret.Description = comment

	for _, tag := range member.Tags() {
		if tag.Key == tagKeyValidate {
			fillValidate(&ret, tag)
			continue
		}
		if len(tag.Options) == 0 || tag.Key != tagKeyForm && tag.Key != tagKeyJson {
			continue
		}

		for _, option := range tag.Options {
			switch {
			case strings.HasPrefix(option, defaultOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Default = segs[1]
				}
			case strings.HasPrefix(option, optionsOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					ret.Enum = strings.Split(segs[1], optionSeparator)
				}
			case strings.HasPrefix(option, rangeOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						ret.Minimum = min
						ret.Maximum = max
					}
				}
			case strings.HasPrefix(option, exampleOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Example = segs[1]
				}
			}
		}
	}

	return ret
}

// https://swagger.io/specification/ Data Types
func primitiveSchema(kind reflect.Kind, t string) (ftype, format string, ok bool) {
	switch kind {
	case reflect.Map:
		return "object", "", true
	case reflect.Int:
		return "integer", "int32", true
	case reflect.Uint:
		return "integer", "uint32", true
	case reflect.Int8:
		return "integer", "int8", true
	case reflect.Uint8:
		return "integer", "uint8", true
	case reflect.Int16:
		return "integer", "int16", true
	case reflect.Uint16:
		return "integer", "uin16", true
	case reflect.Int64:
		return "integer", "int64", true
	case reflect.Uint64:
		return "integer", "uint64", true
	case reflect.Bool:
		return "boolean", "boolean", true
	case reflect.String:
		return "string", "", true
	case reflect.Float32:
		return "number", "float", true
	case reflect.Float64:
		return "number", "double", true
	case reflect.Slice:
		return strings.Replace(t, "[]", "", -1), "", true
	default:
		return "", "", false
	}
}

// StringToBytes converts string to byte slice without a memory allocation.
func stringToBytes(s string) (b []byte) {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}

func countParams(path string) uint16 {
	var n uint16
	s := stringToBytes(path)
	n += uint16(bytes.Count(s, strColon))
	return n
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
