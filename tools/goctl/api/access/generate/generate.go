package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zeromicro/go-zero/tools/goctl/plugin"
)

func Do(filename string, host string, basePath string, in *plugin.Plugin) error {
	access, err := applyGenerate(in, host, basePath)
	if err != nil {
		fmt.Println(err)
	}
	var formatted bytes.Buffer
	enc := json.NewEncoder(&formatted)
	enc.SetIndent("", "  ")

	if err := enc.Encode(access); err != nil {
		fmt.Println(err)
	}

	output := in.Dir + "/" + filename

	err = os.WriteFile(output, formatted.Bytes(), 0666)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("写入access文件完成:%v  err:%v \n", output, err)
	return err
}
