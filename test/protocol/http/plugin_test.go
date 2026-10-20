package http_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/protocol/http/v3"
import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type badAOPController struct{}

func (c *badAOPController) Ping() (any, error) { return "pong", nil }

func (c *badAOPController) Declare(r *Routes) {
	r.Get("/bad", c.Ping).UseGuards("not-a-guard")
}

// 声明里出现无法识别的 AOP 类型时，编译必须失败，而不是静默丢弃。
func TestPlugin_CompileFailsOnUnsupportedAOPType(t *testing.T) {
	ctrl := &badAOPController{}
	provider, err := di.ProviderFromInstance(ctrl, di.Singleton)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	root := module.New(module.Metadata{
		Providers:   []*di.Provider{provider},
		Controllers: []any{ctrl},
	})
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	routes := NewRoutes(reg, ctrl)
	ctrl.Declare(routes)
	defs, err := routes.Defs()
	if err != nil {
		t.Fatalf("declare routes: %v", err)
	}
	for _, def := range defs {
		reg.RegisterDecl(AdapterID, "routes", def)
	}

	if _, err := compiler.Compile(moduleRef, reg, NewPlugin()); err == nil {
		t.Fatalf("expected compile error for unsupported guard type")
	}
}
