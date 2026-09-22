package app

import (
	"time"

	"infinite-canvas/backend/internal/platform"
)

// 画布 Agent 单步的执行边界：一次模型调用最多输出多少 token、最多等多久。
// 两个值都来自运行时策略（管理端可改、环境变量可给部署级默认值），"0"是各自的关闭档位。

// cloudAgentStepOperation 是一次"模型调用"任务的操作名。它同时是任务中心里区分
// "这一条任务是 Agent 的某一步"的依据。
const cloudAgentStepOperation = "cloud_agent_step"

// cloudAgentStepBoostFallbackTokens 是"不限制输出"（策略值为 0）时放大重试用的预算：
// 不限制的本意是"让模型写完"，重试却必须有个上界，否则一次卡住的调用会一直占着单步墙钟。
const cloudAgentStepBoostFallbackTokens = 32_768

// cloudAgentStepLimits 是一次模型调用实际生效的执行边界。
type cloudAgentStepLimits struct {
	// OutputTokens 是本次调用的输出上限；0 表示不限制（只有 Timeout 兜底）。
	OutputTokens int
	// Timeout 是单步墙钟：策略给了秒级值时用它，否则沿用文本任务超时。
	Timeout time.Duration
}

// cloudAgentStepLimits 解析当前生效的单步边界。策略读取失败时退回出厂默认值：
// 拿到一个确定的上界，好过让一次调用无限跑下去。
func (s *Service) cloudAgentStepLimits() cloudAgentStepLimits {
	policy, err := s.runtimeConcurrencySetting()
	if err != nil {
		policy = defaultRuntimePolicy().Task
	}
	limits := cloudAgentStepLimits{OutputTokens: policy.AgentStepMaxOutputTokens}
	if policy.AgentStepTimeoutSeconds > 0 {
		limits.Timeout = time.Duration(policy.AgentStepTimeoutSeconds) * time.Second
	} else {
		limits.Timeout = time.Duration(policy.TextTimeoutMinutes) * time.Minute
	}
	return limits
}

// cloudAgentStepOutputBudget 把生效上限折算成本步请求要带的 maxOutputTokens：
// 放大档（空输出升级重试）在原值上翻倍并以硬上限封顶；原值为 0（不限制）时用兜底值，
// 让重试仍然有界。
func cloudAgentStepOutputBudget(limits cloudAgentStepLimits, boosted bool) int {
	if !boosted {
		return limits.OutputTokens
	}
	if limits.OutputTokens <= 0 {
		return cloudAgentStepBoostFallbackTokens
	}
	return min(limits.OutputTokens*2, platform.MaxRuntimeAgentStepOutputTokens)
}
