// limiter.go 实现 Scheduler adapter 的并发与唯一执行限制逻辑。
package scheduler

// limiterKey 把一个 job 归并到“并发限制”和“指标聚合”使用的稳定 key。
func (a *Adapter) limiterKey(def *JobDefinition) string {
	if def == nil {
		return ""
	}
	return string(def.TriggerKind) + ":" + def.Name
}

// uniqueKey 决定唯一执行锁使用哪个 key。
// 若业务显式给了 UniqueKey，就允许多个不同 job 共用同一把分布式锁语义。
func (a *Adapter) uniqueKey(def *JobDefinition) string {
	if def == nil {
		return ""
	}
	if def.UniqueKey != "" {
		return def.UniqueKey
	}
	return a.limiterKey(def)
}

// ensureLimiter 为某个 job 初始化并发槽通道。
// 默认并发上限为 1，也就是不显式配置时按串行 job 处理。
func (a *Adapter) ensureLimiter(def *JobDefinition) {
	key := a.limiterKey(def)
	if key == "" {
		return
	}
	n := def.MaxConcurrency
	if n <= 0 {
		n = 1
	}
	if _, ok := a.limiters[key]; !ok {
		a.limiters[key] = make(chan struct{}, n)
	}
}

// acquireSlot 决定一次触发能不能真正进入执行。
// 它会同时处理：
// - 最大并发数
// - Unique 唯一执行锁
// - MisfireSkip/queue 两种“槽满时”的策略
func (a *Adapter) acquireSlot(def *JobDefinition) bool {
	key := a.limiterKey(def)
	if key == "" {
		return false
	}
	a.mu.Lock()
	limiter := a.limiters[key]
	a.mu.Unlock()
	if limiter == nil {
		return false
	}
	if def.Unique && a.uniqueLocker != nil && !a.uniqueLocker.TryLock(a.uniqueKey(def)) {
		return false
	}
	if def.MisfirePolicy == MisfireSkip {
		select {
		case limiter <- struct{}{}:
			return true
		default:
			if def.Unique {
				a.clearUnique(def)
			}
			return false
		}
	}
	limiter <- struct{}{}
	return true
}

// releaseSlot 在 job 执行结束后释放并发槽和唯一执行锁。
func (a *Adapter) releaseSlot(def *JobDefinition) {
	key := a.limiterKey(def)
	a.mu.Lock()
	limiter := a.limiters[key]
	a.mu.Unlock()
	if limiter != nil {
		select {
		case <-limiter:
		default:
		}
	}
	if def.Unique {
		a.clearUnique(def)
	}
}

func (a *Adapter) clearUnique(def *JobDefinition) {
	if a.uniqueLocker != nil {
		a.uniqueLocker.Unlock(a.uniqueKey(def))
	}
}
