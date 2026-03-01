package service

import "github.com/kube-rca/backend/internal/model"

// WebhookListNotifier - DB에 저장된 webhook 설정 목록으로 fan-out 전송
type WebhookListNotifier struct {
	delivery *WebhookDeliveryService
}

func NewWebhookListNotifier(delivery *WebhookDeliveryService) *WebhookListNotifier {
	return &WebhookListNotifier{delivery: delivery}
}

func (n *WebhookListNotifier) Notify(alert model.Alert, incidentID string) error {
	if n == nil || n.delivery == nil {
		return nil
	}
	return n.delivery.Deliver(alert, incidentID)
}
