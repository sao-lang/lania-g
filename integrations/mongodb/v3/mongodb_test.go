package mongodb

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type analyticsDB struct{}

func (analyticsDB) MongoDatabaseName() string { return "analytics" }

func TestForRoot_RegistersDatabaseFactoryAndBindings(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatalf("connect mongo client: %v", err)
	}
	db := client.Database("analytics")

	mod, err := ForRoot(Config{
		Name:     "analytics",
		Client:   client,
		DB:       db,
		Database: "analytics",
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	dbToken := reflect.TypeFor[*mongo.Database]()
	clientToken := reflect.TypeFor[*mongo.Client]()
	dbAny, err := mod.Container().Get(dbToken)
	if err != nil {
		t.Fatalf("get db: %v", err)
	}
	if dbAny.(*mongo.Database).Name() != "analytics" {
		t.Fatalf("db name = %q", dbAny.(*mongo.Database).Name())
	}
	clientAny, err := mod.Container().Get(clientToken)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if clientAny.(*mongo.Client) == nil {
		t.Fatalf("client nil")
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.Container = mod.Container().NewChild()

	injected, err := br.Resolve(ctx, nil, reflect.TypeOf(InjectDatabase{}), 0)
	if err != nil {
		t.Fatalf("resolve inject db: %v", err)
	}
	if injected.(InjectDatabase).Database.Name() != "analytics" {
		t.Fatalf("inject database mismatch")
	}

	refValue, err := br.Resolve(ctx, nil, reflect.TypeOf(DatabaseRef[analyticsDB]{}), 1)
	if err != nil {
		t.Fatalf("resolve ref db: %v", err)
	}
	if refValue.(DatabaseRef[analyticsDB]).Database.Name() != "analytics" {
		t.Fatalf("ref database mismatch")
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatalf("connect mongo client: %v", err)
	}
	db := client.Database("analytics")
	mod, err := ForRoot(Config{Name: "analytics", Client: client, DB: db, Database: "analytics"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatalf("connect mongo client: %v", err)
	}
	db := client.Database("analytics")
	mod, err := ForRootCompat(Config{Name: "analytics", Client: client, DB: db, Database: "analytics"})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/mongodb.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
