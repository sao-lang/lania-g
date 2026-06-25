package orm

import (
	stdctx "context"
	"reflect"
	"strings"
	"testing"

	coremodule "github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type testUser struct {
	ID   uint
	Name string
}

type analyticsDB struct{}

func (analyticsDB) ORMDataSourceName() string { return "analytics" }

func TestForRoot_RegistersDBFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	Entity("users", "ID", testUser{})

	db, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}

	mod, err := ForRoot(Config{
		Name: "default",
		DB:   db,
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(*Module).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	dbToken := reflect.TypeFor[*gorm.DB]()
	factoryToken := reflect.TypeFor[Factory]()

	dbAny, err := mod.Container().Get(dbToken)
	if err != nil {
		t.Fatalf("get db: %v", err)
	}
	resolvedDB := dbAny.(*gorm.DB)
	repo := NewRepository[testUser](resolvedDB)
	if repo.DB() == nil {
		t.Fatalf("repo db nil")
	}
	if meta := GetEntity(testUser{}); meta == nil || meta.Table != "users" || meta.PrimaryKey != "ID" {
		t.Fatalf("entity meta=%+v", meta)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, dbToken, 0)
	if err != nil {
		t.Fatalf("resolve db: %v", err)
	}
	if value.(*gorm.DB) != resolvedDB {
		t.Fatalf("resolved db mismatch")
	}

	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	factory := factoryValue.(Factory)
	derived, err := factory.New(Config{
		Dialector:  fakeDialector{},
		GormConfig: &gorm.Config{},
	})
	if err != nil {
		t.Fatalf("factory new: %v", err)
	}
	if derived == nil {
		t.Fatalf("derived db nil")
	}
}

func TestForRoots_RegistersNamedDataSources(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	defaultDB, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open default fake db: %v", err)
	}
	analytics, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open analytics fake db: %v", err)
	}

	mod, err := ForRoots(
		Config{Name: "default", DB: defaultDB},
		Config{Name: "analytics", DB: analytics},
	)
	if err != nil {
		t.Fatalf("for roots: %v", err)
	}
	mod.(*Module).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	if got, ok := mod.(*Module).DataSource("analytics"); !ok || got != analytics {
		t.Fatalf("named datasource mismatch: %v %v", ok, got)
	}
	namedAny, err := mod.Container().Get(DataSourceToken("analytics"))
	if err != nil {
		t.Fatalf("get named db: %v", err)
	}
	if namedAny.(*gorm.DB) != analytics {
		t.Fatalf("named db mismatch")
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(DataSourceRef[analyticsDB]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{
			ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}},
		},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve named datasource ref: %v", err)
	}
	ref := value.(DataSourceRef[analyticsDB])
	if ref.DB != analytics {
		t.Fatalf("resolved named datasource mismatch")
	}
}

func TestMustGetByType_Helper(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	db, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	mod, err := ForRoot(Config{Name: "default", DB: db})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(*Module).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	ref := coremodule.NewModuleRef(mod)
	resolvedDB := coremodule.MustGetByType[*gorm.DB](ref)
	if resolvedDB != db {
		t.Fatalf("typed db mismatch")
	}
	factory := coremodule.MustGetByType[Factory](ref)
	if factory.Default() != db {
		t.Fatalf("typed factory mismatch")
	}
}

type fakeDialector struct{}

func (fakeDialector) Name() string { return "fake" }

func (fakeDialector) Initialize(db *gorm.DB) error { return nil }

func (fakeDialector) Migrator(db *gorm.DB) gorm.Migrator { return migrator.Migrator{} }

func (fakeDialector) DataTypeOf(*schema.Field) string { return "" }

func (fakeDialector) DefaultValueOf(*schema.Field) clause.Expression { return clause.Expr{} }

func (fakeDialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v interface{}) {
	writer.WriteByte('?')
}

func (fakeDialector) QuoteTo(writer clause.Writer, s string) { writer.WriteString(s) }

func (fakeDialector) Explain(sql string, vars ...interface{}) string { return sql }

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	db, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	mod, err := ForRoot(Config{Name: "default", DB: db})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootMultiCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	db, err := gorm.Open(fakeDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	mod, err := ForRootMultiCompat(MultiConfig{DefaultName: "default", DataSources: []Config{{Name: "default", DB: db}}})
	if err != nil {
		t.Fatalf("for root multi compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/orm.ForRootMultiCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
