package service

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"time"

	"github.com/kube-rca/backend/internal/model"
	tmpl "github.com/kube-rca/backend/internal/template"
)

var _ = context.Background // context 패키지 사용 유지 (GetWebhookConfigs에서 사용)

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
func (s *WebhookDeliveryService) Deliver(alert model.Alert, incidentID string) {
	ctx := context.Background()

	// 1. 저장된 webhook configs 조회
	configs, err := s.configDB.GetWebhookConfigs(ctx)
	if err != nil {
		log.Printf("[WebhookDelivery] Failed to load webhook configs: %v", err)
		return
	}
	if len(configs) == 0 {
		return
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
	for _, cfg := range configs {
		if cfg.URL == "" {
			log.Printf("[WebhookDelivery] Skipping config id=%d: URL is empty", cfg.ID)
			continue
		}

		rendered := tmpl.RenderBody(cfg.Body, incidentData, &alertData)

		if err := s.sendHTTP(cfg, rendered); err != nil {
			log.Printf("[WebhookDelivery] Failed to deliver to %s (config id=%d): %v", cfg.URL, cfg.ID, err)
		} else {
			log.Printf("[WebhookDelivery] Delivered to %s (config id=%d)", cfg.URL, cfg.ID)
		}
	}
}

// sendHTTP - 단일 webhook config로 HTTP 요청 전송
func (s *WebhookDeliveryService) sendHTTP(cfg model.WebhookConfig, body string) error {
	req, err := http.NewRequest(cfg.Method, cfg.URL, bytes.NewBufferString(body))
	if err != nil {
		return err
	}

	// Content-Type 기본값 설정 (없으면 application/json)
	hasContentType := false
	for _, h := range cfg.Headers {
		if h.Key != "" {
			req.Header.Set(h.Key, h.Value)
		}
		if http.CanonicalHeaderKey(h.Key) == "Content-Type" {
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
		return http.ErrNotSupported // reuse sentinel; actual code logged above
	}
	return nil
}
