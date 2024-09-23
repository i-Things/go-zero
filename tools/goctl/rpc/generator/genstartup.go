package generator

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	conf "github.com/zeromicro/go-zero/tools/goctl/config"
	"github.com/zeromicro/go-zero/tools/goctl/util"
	"github.com/zeromicro/go-zero/tools/goctl/util/pathx"
)

//go:embed startup.tpl
var startupTemplate string

// GenMain generates the main file of the rpc service, which is an rpc service program call entry
func (g *Generator) GenStartup(ctx DirContext, cfg *conf.Config) error {
	fileName := filepath.Join(ctx.GetStartup().Filename, "init.go")
	imports := make([]string, 0)
	svcImport := fmt.Sprintf(`"%v"`, ctx.GetSvc().Package)
	imports = append(imports, svcImport)
	text, err := pathx.LoadTemplate(category, startupTemplateFile, startupTemplate)
	if err != nil {
		return err
	}

	return util.With("startup").GoFmt(true).Parse(text).SaveTo(map[string]any{
		"imports": strings.Join(imports, pathx.NL),
	}, fileName, false)
}
