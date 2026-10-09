package ai

import (
	"context"
	"fmt"
	"time"
)

type TimeoutError struct {
	Limit      time.Duration
	Connection bool
}

func (e *TimeoutError) Error() string {
	if e.Connection {
		return "连接模型或等待响应超时，未生成新报告。请稍后重试，或联系管理员检查模型服务。"
	}
	return fmt.Sprintf("模型分析超过 %.0f 秒，未生成新报告。可以重新分析，或联系管理员检查模型响应速度与超时配置。", e.Limit.Seconds())
}
func (e *TimeoutError) Unwrap() error { return context.DeadlineExceeded }
func (e *TimeoutError) Code() string  { return "timeout" }
