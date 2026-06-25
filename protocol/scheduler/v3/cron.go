// cron.go 提供 Scheduler adapter 使用的 cron 解析与下一次触发计算辅助。
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// collectJobs 从 registry 收集 job 声明，并按 `trigger:name` 做一次去重。
// 去重主要是保护兼容入口/重复注册场景，避免同一份声明被启动两次。
func collectJobs(reg *registry.Registry) []*JobDefinition {
	if reg == nil {
		return nil
	}
	items := reg.ListDecl(AdapterID, "jobs")
	out := make([]*JobDefinition, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		def, ok := item.(*JobDefinition)
		if !ok || def == nil {
			continue
		}
		key := string(def.TriggerKind) + ":" + def.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, def)
	}
	return out
}

// validateCronExpression 只做轻量语法校验，避免在 Start 阶段启动明显非法的 cron。
func validateCronExpression(expr string) error {
	if everyExpr, ok := strings.CutPrefix(expr, "@every "); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(everyExpr))
		if err != nil || duration <= 0 {
			return fmt.Errorf("invalid @every expression: %s", expr)
		}
		return nil
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		return fmt.Errorf("invalid cron expression: %s", expr)
	}
	return nil
}

// cronSlot 生成一个“当前触发时间槽”的唯一标识。
// 5 段 cron 以分钟为粒度，6 段 cron 以秒为粒度。
func cronSlot(expr string, t time.Time) int64 {
	if len(strings.Fields(expr)) == 6 {
		return t.Unix()
	}
	return t.Truncate(time.Minute).Unix()
}

// matchesCronExpression 判断某个时间点是否命中表达式。
// 当前实现刻意保持简单，只支持 `*`、`*/n`、range、逗号枚举这几类常见写法。
func matchesCronExpression(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		if t.Second() != 0 {
			return false
		}
		return matchCronField(fields[0], t.Minute()) &&
			matchCronField(fields[1], t.Hour()) &&
			matchCronField(fields[2], t.Day()) &&
			matchCronField(fields[3], int(t.Month())) &&
			matchCronField(fields[4], int(t.Weekday()))
	}
	if len(fields) == 6 {
		return matchCronField(fields[0], t.Second()) &&
			matchCronField(fields[1], t.Minute()) &&
			matchCronField(fields[2], t.Hour()) &&
			matchCronField(fields[3], t.Day()) &&
			matchCronField(fields[4], int(t.Month())) &&
			matchCronField(fields[5], int(t.Weekday()))
	}
	return false
}

// matchCronField 是单个 cron 字段的匹配器。
func matchCronField(field string, value int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		if everyPart, ok := strings.CutPrefix(part, "*/"); ok {
			n, err := strconv.Atoi(everyPart)
			if err == nil && n > 0 && value%n == 0 {
				return true
			}
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(rangeParts[0])
				end, err2 := strconv.Atoi(rangeParts[1])
				if err1 == nil && err2 == nil && value >= start && value <= end {
					return true
				}
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil && value == n {
			return true
		}
	}
	return false
}

// estimateNextCronRun 为观测用途估算下一次 cron 命中时间。
// 它通过步进扫描实现，简单直接，但并不追求大规模 cron 调度器那种极致性能。
func estimateNextCronRun(expr string, base time.Time) time.Time {
	if everyExpr, ok := strings.CutPrefix(expr, "@every "); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(everyExpr))
		if err == nil && duration > 0 {
			return base.Add(duration)
		}
		return time.Time{}
	}
	step := time.Minute
	if len(strings.Fields(expr)) == 6 {
		step = time.Second
	}
	cursor := base.Add(step)
	limit := cursor.Add(366 * 24 * time.Hour)
	for !cursor.After(limit) {
		if matchesCronExpression(expr, cursor) {
			return cursor
		}
		cursor = cursor.Add(step)
	}
	return time.Time{}
}
