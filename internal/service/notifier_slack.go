package service

import (
	"log"

	"github.com/kube-rca/backend/internal/client"
	"github.com/kube-rca/backend/internal/model"
)

type slackThreadStore interface {
	GetAlertThreadTS(alertID string) (string, bool)
	UpdateAlertThreadTS(alertID, threadTS string) error
}

// SlackNotifier - Slack Bot 전송과 thread_ts 저장/복원을 담당
type SlackNotifier struct {
	client      *client.SlackClient
	threadStore slackThreadStore
}

func NewSlackNotifier(client *client.SlackClient, threadStore slackThreadStore) *SlackNotifier {
	return &SlackNotifier{
		client:      client,
		threadStore: threadStore,
	}
}

func (n *SlackNotifier) Notify(alert model.Alert, incidentID string) error {
	if n == nil || n.client == nil {
		return nil
	}

	// resolved 알림은 기존 thread_ts를 복원해서 같은 스레드로 전송한다.
	if n.threadStore != nil && alert.Status == "resolved" {
		if threadTS, ok := n.threadStore.GetAlertThreadTS(alert.Fingerprint); ok {
			n.client.StoreThreadTS(alert.Fingerprint, threadTS)
		}
	}

	if err := n.client.SendAlert(alert, alert.Status, incidentID); err != nil {
		return err
	}

	// firing 알림은 새로 발급된 thread_ts를 DB에 저장한다.
	if n.threadStore != nil && alert.Status == "firing" {
		if threadTS, ok := n.client.GetThreadTS(alert.Fingerprint); ok {
			if err := n.threadStore.UpdateAlertThreadTS(alert.Fingerprint, threadTS); err != nil {
				log.Printf("Failed to save thread_ts to DB: %v", err)
			}
		}
	}

	return nil
}
