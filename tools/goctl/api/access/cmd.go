package access

import (
	"github.com/spf13/cobra"
	"github.com/zeromicro/go-zero/tools/goctl/api/access/generate"
	"github.com/zeromicro/go-zero/tools/goctl/api/parser"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
	"github.com/zeromicro/go-zero/tools/goctl/util/pathx"
	"path/filepath"
)

var (
	// VarStringOutput describes the output.
	VarStringFilename string
	// VarStringHome describes the goctl home.
	VarStringBasepath string
	// VarStringRemote describes the remote git repository.
	VarStringHost string
	// VarStringBranch describes the git branch.
	VarStringDir string
	// VarStringAPI describes an API file.
	VarStringAPI string
)

func AccessCommand(_ *cobra.Command, _ []string) error {

	if len(VarStringFilename) == 0 {
		VarStringFilename = "access.json"
	}
	apiPath := VarStringAPI
	var p plugin.Plugin
	if len(apiPath) > 0 && pathx.FileExists(apiPath) {
		api, err := parser.Parse(apiPath)
		if err != nil {
			return err
		}

		p.Api = api
	}

	absApiFilePath, err := filepath.Abs(apiPath)
	if err != nil {
		return err
	}

	p.ApiFilePath = absApiFilePath
	dirAbs, err := filepath.Abs(VarStringDir)
	if err != nil {
		return err
	}

	p.Dir = dirAbs
	api, err := parser.Parse(p.ApiFilePath)
	if err != nil {
		return err
	}
	p.Api = api
	return generate.Do(VarStringFilename, VarStringHost, VarStringBasepath, &p)
}
