package rpc

// 几乎所有代码由欧阳完成，我只是搬运过来。

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
)

var (
	rootPkg, server, service, version string

	twirpFile, serverFile, rpcPkg string
)

func init() {
	Cmd.Flags().StringVar(&server, "server", "", "服务")
	Cmd.Flags().StringVar(&service, "service", "", "子服务")
	Cmd.Flags().StringVar(&version, "version", "1", "版本")

	Cmd.MarkFlagRequired("server")
}

func getModuleName() string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		panic(err)
	}

	f, err := modfile.Parse("", b, nil)
	if err != nil {
		panic(err)
	}

	return f.Module.Mod.Path
}

// Cmd 接口生成工具
var Cmd = &cobra.Command{
	Use:   "rpc",
	Short: "生成 rpc 接口",
	Long: `脚手架功能：
- 生成 rpc/**/*.proto 模版
- 生成 rpc/**/*.go
- 生成 rpc/**/*.pb.go
- 生成 rpc/**/*.twirp.go
- 注册接口到 http server`,
	Run: func(cmd *cobra.Command, args []string) {
		if isSniperDir() {
			color.Red("sniper rpc 只能在 sniper 项目根目录运行!")
			os.Exit(1)
		}

		if service == "" {
			service = server
		}
		rootPkg = getModuleName()

		genRPC()
		genImplements()
		registerServer()
	},
}

func isSniperDir() bool {
	dirs, err := os.ReadDir(".")
	if err != nil {
		panic(err)
	}

	// 检查 sniper 项目目录结构
	// sniper 项目依赖 cmd/pkg/rpc 三个目录
	sniperDirs := map[string]bool{"cmd": true, "pkg": true, "rpc": true}

	c := 0
	for _, d := range dirs {
		if sniperDirs[d.Name()] {
			c++
		}
	}

	return c != len(sniperDirs)
}

func genRPC() {
	proto := fmt.Sprintf("rpc/%s/v%s/%s.proto", server, version, service)
	if !fileExists(proto) {
		genProto(proto)
	}

	cmd := exec.Command("make", "rpc")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func registerServer() {
	httpFile := "cmd/http/http.go"
	fset := token.NewFileSet()
	httpAST, err := decorator.ParseFile(fset, httpFile, nil, parser.ParseComments)
	if err != nil {
		return
	}

	// 检查 import 包
	if !serverImported(httpAST.Imports) {
		genPKGTemplate()
		appendImportPKGs(httpAST)
	}
	// 处理注册路由
	for _, decl := range httpAST.Decls {
		gen, ok := decl.(*dst.FuncDecl)
		if !ok {
			continue
		}
		if gen.Name.Name != "initMux" {
			continue
		}

		if serverRegistered(gen) {
			return
		}

		genServerTemplate()
		appendServer(gen)
	}

	f, err := os.OpenFile(httpFile, os.O_WRONLY|os.O_CREATE, 0766)
	if err != nil {
		return
	}
	defer f.Close()
	if err := decorator.Fprint(f, httpAST); err != nil {
		panic(err)
	}
}

func genImplements() {
	twirpFile = fmt.Sprintf("rpc/%s/v%s/%s.twirp.go", server, version, service)
	serverFile = fmt.Sprintf("rpc/%s/v%s/%s.go", server, version, service)

	rpcPkg = fmt.Sprintf("%s/rpc/%s/v%s", rootPkg, server, version)

	if !fileExists(twirpFile) {
		panic("twirp file does not exist: " + twirpFile)
	}

	genOrUpdateTwirpServer()
}

func createDirAndFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	return os.Create(path)
}

func parseAST(file string) (*ast.File, *token.FileSet) {
	b, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	a, err := parser.ParseFile(fset, "", string(b), parser.ParseComments)
	if err != nil {
		panic(err)
	}

	return a, fset
}

func upper1st(s string) string {
	if len(s) == 0 {
		return s
	}

	r := []rune(s)

	if r[0] >= 97 && r[0] <= 122 {
		r[0] -= 32 // 大小写字母ASCII值相差32位
	}

	return string(r)
}
