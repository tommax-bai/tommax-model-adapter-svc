// Package core 定义 model-adapter 的统一内部协议（docs/03 §1.4）。
// provider 插件只依赖本包；新增供应商 = 新增 internal/provider/<vendor> 包并注册，不改既有代码。
package core

import (
	"context"
	"time"
)

// Capability 与 proto 的 Capability 对齐（字符串形式便于配置与日志）。
type Capability string

const (
	CapImageText2Img Capability = "image.text2img"
	CapImageRef2Img  Capability = "image.ref2img"
	CapVideoText2Vid Capability = "video.text2video"
	CapVideoImg2Vid  Capability = "video.img2video"
)

// JobStatus 的失败分类供 generation 做重试决策，provider 必须如实归类。
type JobStatus string

const (
	StatusRunning         JobStatus = "RUNNING"
	StatusSucceeded       JobStatus = "SUCCEEDED"
	StatusFailedRetryable JobStatus = "FAILED_RETRYABLE"
	StatusFailedPermanent JobStatus = "FAILED_PERMANENT"
	StatusContentBlocked  JobStatus = "CONTENT_BLOCKED"
)

// Request 是统一推理请求（供应商无关）。
type Request struct {
	TaskID        string            // 幂等键
	ProviderModel string            // "<provider>/<model>"，router 已拆解出 Model 字段
	Model         string            // 供应商内部模型名
	Capability    Capability
	Prompt        string
	RefURLs       []string
	Params        map[string]string
}

// Output 是统一产物：URL 与 Data 二选一。
type Output struct {
	URL      string
	Data     []byte
	MimeType string
	Width    int
	Height   int
}

// Result 是作业当前快照。
type Result struct {
	Status   JobStatus
	Progress int
	Outputs  []Output
	ErrorMsg string
}

// Job 是 jobstore 中的作业记录。
type Job struct {
	ID        string
	TaskID    string
	Provider  string
	Request   Request
	Result    Result
	CreatedAt time.Time
}

// Provider 是供应商插件接口。实现必须并发安全。
type Provider interface {
	// Name 返回注册名（provider_model 的前半段）。
	Name() string
	// Submit 启动作业（异步）；实现负责把最终结果写入 job（经 UpdateFn）。
	Submit(ctx context.Context, job *Job, update UpdateFn) error
	// Cancel 尽力取消。
	Cancel(ctx context.Context, job *Job) error
}

// UpdateFn 由 jobstore 提供给 provider 回写结果（避免 provider 直接依赖存储实现）。
type UpdateFn func(jobID string, result Result)
