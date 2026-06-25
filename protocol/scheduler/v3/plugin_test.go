package scheduler

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	schedulerbinding "github.com/sao-lang/lania-g/protocol/scheduler/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	schedulerprotocol "github.com/sao-lang/lania-g/protocol/scheduler/v3/protocol"
)

type testSchedulerService struct{}

var (
	afterHits  atomic.Int32
	everyHits  atomic.Int32
	retryHits  atomic.Int32
	overlapMax atomic.Int32
	overlapCur atomic.Int32
	observedCh chan string
)

func (s *testSchedulerService) RunAfter(
	ctx stdctx.Context,
	name schedulerbinding.JobName,
	trigger schedulerbinding.TriggerType,
) error {
	if ctx == nil || string(name) != "after-job" || string(trigger) != "after" {
		return stdctx.Canceled
	}
	afterHits.Add(1)
	select {
	case observedCh <- "after":
	default:
	}
	return nil
}

func (s *testSchedulerService) RunEvery(
	ctx stdctx.Context,
	name schedulerbinding.JobName,
	trigger schedulerbinding.TriggerType,
) error {
	if ctx == nil || string(name) != "every-job" || string(trigger) != "every" {
		return stdctx.Canceled
	}
	everyHits.Add(1)
	select {
	case observedCh <- "every":
	default:
	}
	return nil
}

func (s *testSchedulerService) RunRetry(ctx stdctx.Context) error {
	_ = ctx
	n := retryHits.Add(1)
	if n < 3 {
		return errors.New("retry me")
	}
	return nil
}

func (s *testSchedulerService) RunUnique(ctx stdctx.Context) error {
	_ = ctx
	current := overlapCur.Add(1)
	for {
		max := overlapMax.Load()
		if current <= max || overlapMax.CompareAndSwap(max, current) {
			break
		}
	}
	time.Sleep(60 * time.Millisecond)
	overlapCur.Add(-1)
	return nil
}

type testHost struct {
	rt        *runtime.Runtime
	reg       *registry.Registry
	moduleRef *module.ModuleRef
}

func (h *testHost) Runtime() *runtime.Runtime    { return h.rt }
func (h *testHost) Registry() *registry.Registry { return h.reg }
func (h *testHost) ModuleRef() *module.ModuleRef { return h.moduleRef }

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	svc := &testSchedulerService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Job("after-job", svc).After(20*time.Millisecond, svc.RunAfter).Build()

	_, err := (&Plugin{}).Scan(moduleRef, nil)
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile with explicit registry: %v", err)
	}
	if compiled == nil {
		t.Fatalf("expected compiled app")
	}
}

func TestSchedulerAdapter_CompileAndTrigger(t *testing.T) {
	afterHits.Store(0)
	everyHits.Store(0)
	observedCh = make(chan string, 8)
	defer func() { observedCh = nil }()

	svc := &testSchedulerService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Job("after-job", svc).After(20*time.Millisecond, svc.RunAfter).Build()
	NewAPI(reg).Job("every-job", svc).Every(40*time.Millisecond, svc.RunEvery).Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	adp := New()
	host := &testHost{rt: rt, reg: reg, moduleRef: moduleRef}
	if err := adp.Mount(host); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if err := adp.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer adp.Stop()

	deadline := time.After(1500 * time.Millisecond)
	gotAfter := false
	gotEvery := false
	for !(gotAfter && gotEvery) {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting triggers: after=%d every=%d", afterHits.Load(), everyHits.Load())
		case kind := <-observedCh:
			if kind == "after" {
				gotAfter = true
			}
			if kind == "every" {
				gotEvery = true
			}
		}
	}
}

func TestSchedulerRouteKey(t *testing.T) {
	key := runtime.BuildRouteKey(schedulerprotocol.Protocol, string(TriggerCron), "nightly")
	if key != "scheduler:cron:nightly" {
		t.Fatalf("route key=%s", key)
	}
}

func TestSchedulerBuilder_AdvancedOptions(t *testing.T) {
	svc := &testSchedulerService{}
	def := NewAPI(registry.New()).
		Job("advanced", svc).
		Every(time.Second, svc.RunEvery).
		Retry(2, 10*time.Millisecond).
		MaxConcurrency(3).
		Unique("advanced-key").
		WithTimeout(time.Second).
		Misfire(MisfireSkip).
		Build()
	if def == nil {
		t.Fatalf("definition is nil")
	}
	if def.RetryAttempts != 2 || def.RetryBackoff != 10*time.Millisecond {
		t.Fatalf("retry=%d backoff=%s", def.RetryAttempts, def.RetryBackoff)
	}
	if def.MaxConcurrency != 3 || !def.Unique || def.UniqueKey != "advanced-key" {
		t.Fatalf("advanced fields=%+v", def)
	}
	if def.Timeout != time.Second || def.MisfirePolicy != MisfireSkip {
		t.Fatalf("timeout/misfire=%+v", def)
	}
}

func TestSchedulerAdapter_RetryAndUnique(t *testing.T) {
	retryHits.Store(0)
	overlapCur.Store(0)
	overlapMax.Store(0)

	svc := &testSchedulerService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Job("retry-job", svc).After(10*time.Millisecond, svc.RunRetry).Retry(2, 5*time.Millisecond).Build()
	NewAPI(reg).Job("unique-job", svc).Every(10*time.Millisecond, svc.RunUnique).MaxConcurrency(2).Unique().Misfire(MisfireSkip).Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}
	adp := New()
	host := &testHost{rt: rt, reg: reg, moduleRef: moduleRef}
	if err := adp.Mount(host); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if err := adp.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer adp.Stop()

	time.Sleep(250 * time.Millisecond)
	if retryHits.Load() != 3 {
		t.Fatalf("retry hits=%d", retryHits.Load())
	}
	if overlapMax.Load() > 1 {
		t.Fatalf("unique overlap max=%d", overlapMax.Load())
	}
	snapshots := adp.Snapshot()
	if len(snapshots) == 0 {
		t.Fatalf("expected snapshots")
	}
	retrySnap := snapshots["after:retry-job"]
	if retrySnap.Metrics.TotalRuns == 0 || retrySnap.Metrics.SuccessRuns == 0 {
		t.Fatalf("retry metrics=%+v", retrySnap.Metrics)
	}
	if len(retrySnap.RecentRuns) == 0 || retrySnap.RecentRuns[0].Attempts != 3 {
		t.Fatalf("retry history=%+v", retrySnap.RecentRuns)
	}
	uniqueSnap := snapshots["every:unique-job"]
	if uniqueSnap.Metrics.NextRunAt.IsZero() {
		t.Fatalf("unique next run missing: %+v", uniqueSnap.Metrics)
	}
}

type httpMountHost struct {
	pattern string
	handler http.Handler
}

func (h *httpMountHost) MountHTTP(pattern string, handler http.Handler) error {
	h.pattern = pattern
	h.handler = handler
	return nil
}

func TestSchedulerHTTPBridge(t *testing.T) {
	adp := New()
	host := &httpMountHost{}
	if err := MountHTTPBridge(host, adp, "/scheduler/status"); err != nil {
		t.Fatalf("mount bridge: %v", err)
	}
	if host.pattern != "/scheduler/status" || host.handler == nil {
		t.Fatalf("host=%+v", host)
	}
	req := httptest.NewRequest(http.MethodGet, "/scheduler/status", nil)
	rec := httptest.NewRecorder()
	host.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var snapshots map[string]JobSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
}

func TestSchedulerPlugin_OwnerErrorsUseUnifiedMeta(t *testing.T) {
	receiver := &testSchedulerService{}
	root := module.CreateModule(nil, nil, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}

	reg := registry.New()
	NewAPI(reg).Job("missing-job", receiver).After(time.Second, receiver.RunRetry).Build()

	_, err := compiler.Compile(module.NewModuleRef(root), reg, NewPlugin())
	if err == nil {
		t.Fatalf("expected owner missing error")
	}

	var kernelErr *kerrors.KernelError
	if !errors.As(err, &kernelErr) {
		t.Fatalf("expected KernelError, got %T", err)
	}
	if got := kernelErr.Meta["ownerKind"]; got != "receiver" {
		t.Fatalf("ownerKind = %v, want receiver", got)
	}
	if got := kernelErr.Meta["ownerStatus"]; got != "missing" {
		t.Fatalf("ownerStatus = %v, want missing", got)
	}
	if got := kernelErr.Meta["ownerToken"]; got == "" {
		t.Fatalf("ownerToken = %v, want non-empty", got)
	}
	candidates, ok := kernelErr.Meta["ownerCandidates"].([]string)
	if !ok || len(candidates) != 0 {
		t.Fatalf("ownerCandidates = %#v, want empty slice", kernelErr.Meta["ownerCandidates"])
	}
}

var _ adapter.HTTPMountHost = (*httpMountHost)(nil)
