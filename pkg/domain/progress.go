package domain

// ProgressFunc 长任务进度回调（stage: 下载/校验/解压…）。
// 由 service 与 installer 共用，确保 CLI 与 GUI 使用同一接口。
type ProgressFunc func(done, total int64, stage string)
