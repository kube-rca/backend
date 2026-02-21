// Package template provides webhook body template rendering.
//
// 지원하는 변수 형식:
//
//	{{incident.id}}, {{incident.title}}, {{incident.severity}},
//	{{incident.status}}, {{incident.created_at}}, {{incident.summary}}
//
//	{{alert.alertname}}, {{alert.severity}}, {{alert.namespace}},
//	{{alert.status}}, {{alert.description}}, {{alert.summary}},
//	{{alert.started_at}}, {{alert.ended_at}}, {{alert.fingerprint}}
package template

import (
	"strings"
	"time"

	"github.com/kube-rca/backend/internal/model"
)

// IncidentData - 템플릿 렌더링에 사용할 Incident 데이터
type IncidentData struct {
	ID        string
	Title     string
	Severity  string
	Status    string
	CreatedAt time.Time
	Summary   string
}

// AlertData - 템플릿 렌더링에 사용할 Alert 데이터
type AlertData struct {
	AlertName   string
	Severity    string
	Namespace   string
	Status      string
	Description string
	Summary     string
	StartedAt   time.Time
	EndedAt     time.Time
	Fingerprint string
}

// IncidentDataFromDetail - IncidentDetailResponse에서 IncidentData 생성
func IncidentDataFromDetail(inc *model.IncidentDetailResponse) IncidentData {
	summary := ""
	if inc.AnalysisSummary != nil {
		summary = *inc.AnalysisSummary
	}
	return IncidentData{
		ID:        inc.IncidentID,
		Title:     inc.Title,
		Severity:  inc.Severity,
		Status:    inc.Status,
		CreatedAt: inc.FiredAt,
		Summary:   summary,
	}
}

// AlertDataFromModel - model.Alert에서 AlertData 생성
func AlertDataFromModel(alert model.Alert) AlertData {
	return AlertData{
		AlertName:   alert.Labels["alertname"],
		Severity:    alert.Labels["severity"],
		Namespace:   alert.Labels["namespace"],
		Status:      alert.Status,
		Description: alert.Annotations["description"],
		Summary:     alert.Annotations["summary"],
		StartedAt:   alert.StartsAt,
		EndedAt:     alert.EndsAt,
		Fingerprint: alert.Fingerprint,
	}
}

// RenderBody - webhook body 템플릿의 변수를 실제 값으로 치환
//
// incident 또는 alert 중 하나만 전달해도 동작합니다.
// nil로 전달된 항목의 변수는 빈 문자열로 치환됩니다.
func RenderBody(body string, incident *IncidentData, alert *AlertData) string {
	pairs := make([]string, 0, 28)

	// --- Incident 변수 ---
	if incident != nil {
		pairs = append(pairs,
			"{{incident.id}}", incident.ID,
			"{{incident.title}}", incident.Title,
			"{{incident.severity}}", incident.Severity,
			"{{incident.status}}", incident.Status,
			"{{incident.created_at}}", incident.CreatedAt.Format(time.RFC3339),
			"{{incident.summary}}", incident.Summary,
		)
	} else {
		pairs = append(pairs,
			"{{incident.id}}", "",
			"{{incident.title}}", "",
			"{{incident.severity}}", "",
			"{{incident.status}}", "",
			"{{incident.created_at}}", "",
			"{{incident.summary}}", "",
		)
	}

	// --- Alert 변수 ---
	if alert != nil {
		endedAt := ""
		if !alert.EndedAt.IsZero() {
			endedAt = alert.EndedAt.Format(time.RFC3339)
		}
		pairs = append(pairs,
			"{{alert.alertname}}", alert.AlertName,
			"{{alert.severity}}", alert.Severity,
			"{{alert.namespace}}", alert.Namespace,
			"{{alert.status}}", alert.Status,
			"{{alert.description}}", alert.Description,
			"{{alert.summary}}", alert.Summary,
			"{{alert.started_at}}", alert.StartedAt.Format(time.RFC3339),
			"{{alert.ended_at}}", endedAt,
			"{{alert.fingerprint}}", alert.Fingerprint,
		)
	} else {
		pairs = append(pairs,
			"{{alert.alertname}}", "",
			"{{alert.severity}}", "",
			"{{alert.namespace}}", "",
			"{{alert.status}}", "",
			"{{alert.description}}", "",
			"{{alert.summary}}", "",
			"{{alert.started_at}}", "",
			"{{alert.ended_at}}", "",
			"{{alert.fingerprint}}", "",
		)
	}

	return strings.NewReplacer(pairs...).Replace(body)
}
