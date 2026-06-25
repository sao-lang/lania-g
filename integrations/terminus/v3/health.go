// health.go 实现 terminus 集成的健康检查聚合与状态计算逻辑。
package terminus

import (
	"sync"
	"time"
)

// HealthStatus 表示健康检查的聚合状态。
type HealthStatus string

const (
	// StatusPass 表示检查通过。
	StatusPass HealthStatus = "pass"
	// StatusWarn 表示检查存在告警但未到失败级别。
	StatusWarn HealthStatus = "warn"
	// StatusFail 表示检查失败。
	StatusFail HealthStatus = "fail"
)

// HealthCheckResult 表示一次健康检查的总结果。
type HealthCheckResult struct {
	Status    HealthStatus            `json:"status"`
	Checks    map[string]*CheckResult `json:"checks,omitempty"`
	Version   string                  `json:"version,omitempty"`
	ReleaseID string                  `json:"releaseId,omitempty"`
}

// CheckResult 表示单个指标的检查结果。
type CheckResult struct {
	Status          HealthStatus `json:"status"`
	ObservedValue   interface{}  `json:"observedValue,omitempty"`
	ObservedUnit    string       `json:"observedUnit,omitempty"`
	Output          string       `json:"output,omitempty"`
	ComponentType   string       `json:"componentType,omitempty"`
	ComponentID     string       `json:"componentId,omitempty"`
	MeasurementName string       `json:"measurementName,omitempty"`
	Time            time.Time    `json:"time,omitempty"`
}

// Indicator 定义健康检查指标需要实现的最小能力。
type Indicator interface {
	Name() string
	Check() (*CheckResult, error)
}

// HealthService 负责收集指标并产出最终健康检查结果。
type HealthService struct {
	mu         sync.RWMutex
	indicators []Indicator
	version    string
	releaseID  string
}

// NewHealthService 创建一个空的 HealthService。
func NewHealthService() *HealthService {
	return &HealthService{indicators: make([]Indicator, 0)}
}

// SetVersion 设置健康检查响应中的版本信息。
func (s *HealthService) SetVersion(version string) *HealthService {
	s.version = version
	return s
}

// SetReleaseID 设置健康检查响应中的发布标识。
func (s *HealthService) SetReleaseID(releaseID string) *HealthService {
	s.releaseID = releaseID
	return s
}

// AddIndicator 追加一个健康检查指标。
func (s *HealthService) AddIndicator(indicator Indicator) *HealthService {
	if indicator == nil {
		return s
	}
	s.mu.Lock()
	s.indicators = append(s.indicators, indicator)
	s.mu.Unlock()
	return s
}

// Check 依次执行所有指标并汇总最终健康状态。
func (s *HealthService) Check() *HealthCheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &HealthCheckResult{
		Status:    StatusPass,
		Checks:    make(map[string]*CheckResult),
		Version:   s.version,
		ReleaseID: s.releaseID,
	}
	for _, indicator := range s.indicators {
		checkResult, err := indicator.Check()
		if err != nil {
			checkResult = &CheckResult{
				Status: StatusFail,
				Output: err.Error(),
				Time:   time.Now(),
			}
		}
		if checkResult.Time.IsZero() {
			checkResult.Time = time.Now()
		}
		result.Checks[indicator.Name()] = checkResult
		if checkResult.Status == StatusFail {
			result.Status = StatusFail
		} else if checkResult.Status == StatusWarn && result.Status == StatusPass {
			result.Status = StatusWarn
		}
	}
	return result
}
