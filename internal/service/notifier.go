package service

import (
	"errors"
	"fmt"

	"github.com/kube-rca/backend/internal/model"
)

// AlertNotifier - 채널별 알림 전송 구현체를 위한 공통 인터페이스
type AlertNotifier interface {
	Notify(alert model.Alert, incidentID string) error
}

// CompositeAlertNotifier - 여러 notifier를 순차 실행하는 fan-out notifier
type CompositeAlertNotifier struct {
	notifiers []AlertNotifier
}

func NewCompositeAlertNotifier(notifiers ...AlertNotifier) *CompositeAlertNotifier {
	filtered := make([]AlertNotifier, 0, len(notifiers))
	for _, notifier := range notifiers {
		if notifier == nil {
			continue
		}
		filtered = append(filtered, notifier)
	}

	return &CompositeAlertNotifier{notifiers: filtered}
}

func (n *CompositeAlertNotifier) Notify(alert model.Alert, incidentID string) error {
	if n == nil || len(n.notifiers) == 0 {
		return nil
	}

	errs := make([]error, 0)
	for i, notifier := range n.notifiers {
		if err := notifier.Notify(alert, incidentID); err != nil {
			errs = append(errs, fmt.Errorf("notifier[%d]: %w", i, err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
