package notify

import "github.com/NamiraNet/namira-core/internal/core"

type Notifier interface {
	Send(result core.CheckResult) error
	SendWithQRCode(result core.CheckResult) error
}
