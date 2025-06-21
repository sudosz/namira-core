package notify

import "github.com/NaMiraNet/namira-core/internal/core"

type Notifier interface {
	Send(result core.CheckResult) error
}
