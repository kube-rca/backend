package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kube-rca/backend/internal/model"
	tmpl "github.com/kube-rca/backend/internal/template"
)

type webhookType string

const (
	webhookTypeSlack webhookType = "slack"
	webhookTypeTeams webhookType = "teams"
	webhookTypeHTTP  webhookType = "http"

	webhookTypeHeader = "X-Webhook-Type"
)

// webhookConfigReader - DB 인터페이스 (delivery 전용)
type webhookConfigReader interface {
	GetWebhookConfigs(ctx context.Context) ([]model.WebhookConfig, error)
	GetWebhookConfigByID(ctx context.Context, id int) (*model.WebhookConfig, error)
}

// incidentReader - incident 조회용 DB 인터페이스
type incidentReader interface {
	GetIncidentDetail(id string) (*model.IncidentDetailResponse, error)
}

// WebhookDeliveryService - 사용자 설정 Webhook으로 알람을 전송하는 서비스
type WebhookDeliveryService struct {
	configDB   webhookConfigReader
	incidentDB incidentReader
	httpClient *http.Client
}

// NewWebhookDeliveryService 생성자
func NewWebhookDeliveryService(configDB webhookConfigReader, incidentDB incidentReader) *WebhookDeliveryService {
	return &WebhookDeliveryService{
		configDB:   configDB,
		incidentDB: incidentDB,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Deliver - 저장된 모든 webhook config에 렌더링된 body를 HTTP로 전송
//
// 기존 Slack 전송과 독립적으로 동작합니다.
// 개별 config 실패 시 로그만 남기고 나머지는 계속 전송합니다.
func (s *WebhookDeliveryService) Deliver(alert model.Alert, incidentID string) error {
	ctx := context.Background()

	// 1. 저장된 webhook configs 조회
	configs, err := s.configDB.GetWebhookConfigs(ctx)
	if err != nil {
		log.Printf("[WebhookDelivery] Failed to load webhook configs: %v", err)
		return err
	}
	if len(configs) == 0 {
		return nil
	}

	// 2. Incident 상세 조회 (템플릿에 주입 – 실패해도 nil로 진행)
	var incidentData *tmpl.IncidentData
	if incidentID != "" {
		inc, err := s.incidentDB.GetIncidentDetail(incidentID)
		if err != nil {
			log.Printf("[WebhookDelivery] Failed to load incident detail (id=%s): %v", incidentID, err)
		} else {
			d := tmpl.IncidentDataFromDetail(inc)
			incidentData = &d
		}
	}

	// Alert 데이터 변환
	alertData := tmpl.AlertDataFromModel(alert)

	// 3. 각 config에 대해 렌더링 후 HTTP 전송
	deliveryErrors := make([]error, 0)
	for _, cfg := range configs {
		if cfg.URL == "" {
			log.Printf("[WebhookDelivery] Skipping config id=%d: URL is empty", cfg.ID)
			continue
		}

		bodyTemplate := resolveBodyTemplate(cfg)
		rendered := tmpl.RenderBody(bodyTemplate, incidentData, &alertData)

		if err := s.sendHTTP(cfg, rendered); err != nil {
			log.Printf("[WebhookDelivery] Failed to deliver to %s (config id=%d): %v", cfg.URL, cfg.ID, err)
			deliveryErrors = append(deliveryErrors, fmt.Errorf("config id=%d: %w", cfg.ID, err))
		} else {
			log.Printf("[WebhookDelivery] Delivered to %s (config id=%d)", cfg.URL, cfg.ID)
		}
	}

	if len(deliveryErrors) > 0 {
		return errors.Join(deliveryErrors...)
	}
	return nil
}

// sendHTTP - 단일 webhook config로 HTTP 요청 전송
func (s *WebhookDeliveryService) sendHTTP(cfg model.WebhookConfig, body string) error {
	method := strings.TrimSpace(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, cfg.URL, bytes.NewBufferString(body))
	if err != nil {
		return err
	}

	// Content-Type 기본값 설정 (없으면 application/json)
	hasContentType := false
	for _, h := range cfg.Headers {
		key := strings.TrimSpace(h.Key)
		if key == "" {
			continue
		}

		// 내부 관리용 메타데이터 헤더는 외부 webhook으로 전송하지 않는다.
		if strings.EqualFold(key, webhookTypeHeader) {
			continue
		}

		req.Header.Set(key, h.Value)
		if http.CanonicalHeaderKey(key) == "Content-Type" {
			hasContentType = true
		}
	}
	if !hasContentType {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func resolveBodyTemplate(cfg model.WebhookConfig) string {
	if t, ok := webhookTypeFromHeaders(cfg.Headers); ok {
		return defaultTemplateByType(t)
	}

	// 구버전 데이터 호환: 타입 헤더가 없고 body가 채워져 있으면 기존 body를 그대로 사용
	if strings.TrimSpace(cfg.Body) != "" {
		return cfg.Body
	}

	return defaultTemplateByType(webhookTypeHTTP)
}

func webhookTypeFromHeaders(headers []model.WebhookHeader) (webhookType, bool) {
	for _, h := range headers {
		if !strings.EqualFold(strings.TrimSpace(h.Key), webhookTypeHeader) {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(h.Value)) {
		case string(webhookTypeSlack):
			return webhookTypeSlack, true
		case string(webhookTypeTeams):
			return webhookTypeTeams, true
		case string(webhookTypeHTTP):
			return webhookTypeHTTP, true
		default:
			return webhookTypeHTTP, true
		}
	}

	return "", false
}

func defaultTemplateByType(t webhookType) string {
	switch t {
	case webhookTypeSlack:
		return `{
  "text": "[{{alert.status}}] {{alert.alertname}} ({{alert.severity}})\n{{incident.title}}\n{{alert.summary}}"
}`
	case webhookTypeTeams:
		return `{
  "@type": "MessageCard",
  "@context": "https://schema.org/extensions",
  "summary": "{{incident.title}}",
  "themeColor": "0076D7",
  "title": "[{{alert.status}}] {{alert.alertname}}",
  "text": "{{alert.summary}}"
}`
	default:
		return `{
  "incident": {
    "id": "{{incident.id}}",
    "title": "{{incident.title}}",
    "severity": "{{incident.severity}}",
    "status": "{{incident.status}}",
    "created_at": "{{incident.created_at}}",
    "summary": "{{incident.summary}}"
  },
  "alert": {
    "alertname": "{{alert.alertname}}",
    "severity": "{{alert.severity}}",
    "namespace": "{{alert.namespace}}",
    "status": "{{alert.status}}",
    "description": "{{alert.description}}",
    "summary": "{{alert.summary}}",
    "started_at": "{{alert.started_at}}",
    "ended_at": "{{alert.ended_at}}",
    "fingerprint": "{{alert.fingerprint}}"
  }
}`
	}
}
