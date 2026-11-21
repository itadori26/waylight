package grafana

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"

	"waylight/internal/modules/grafana/domain"

	"waylight/internal/modules"
	"waylight/internal/modules/grafana/adapter/controller"
	"waylight/internal/modules/grafana/adapter/gateway"
	"waylight/internal/modules/grafana/adapter/smartsearch"
	"waylight/internal/modules/grafana/usecase"
	"waylight/internal/shared/answers"
	"waylight/internal/shared/kernel"
)

// Module wires the Grafana rings and implements modules.Module.
type Module struct {
	modules.Base
	handlers *controller.Handlers
	uc       usecase.Grafana
}

// New builds the Grafana module (#574); cipher encrypts the token.
func New(db *sql.DB, cipher gateway.Cipher, client *http.Client, clock kernel.Clock) *Module {
	uc := usecase.New(gateway.NewSettingsRepository(db, cipher), gateway.NewAPI(client), clock)
	return &Module{handlers: controller.NewHandlers(uc), uc: uc}
}

// Name implements modules.Module.
func (m *Module) Name() string { return "grafana" }

// DataKind declares who owns the module's data (ADR 0018).
func (m *Module) DataKind() modules.DataKind { return modules.InstanceData }

// Migrations implements modules.Module.
func (m *Module) Migrations() fs.FS { return gateway.Migrations() }

// WidgetKinds implements modules.Module.
func (m *Module) WidgetKinds() []string { return []string{"grafana-panel", "grafana-alerts"} }

// GuestGatedRoutes implements modules.GuestGated: guests read alerts and
// panels only with the matching widget on their tab, and never browse the
// dashboards (no widget has the kind "grafana-browse").
func (m *Module) GuestGatedRoutes() map[string]string {
	return map[string]string{
		"/api/grafana/alerts":     "grafana-alerts",
		"/api/grafana/render":     "grafana-panel",
		"/api/grafana/data":       "grafana-panel",
		"/api/grafana/dashboards": "grafana-browse",
	}
}

// RegisterRoutes implements modules.Module.
func (m *Module) RegisterRoutes(r modules.Router) {
	h := m.handlers
	r.Protected("GET /api/grafana/settings", h.Settings)
	r.Protected("PUT /api/grafana/settings", h.SaveSettings)
	r.Protected("GET /api/grafana/alerts", h.Alerts)
	r.Protected("GET /api/grafana/dashboards", h.Dashboards)
	r.Protected("GET /api/grafana/dashboards/{uid}/panels", h.Panels)
	r.Protected("GET /api/grafana/render/{uid}/{panel}", h.Render)
	r.Protected("GET /api/grafana/data/{uid}/{panel}", h.PanelData)
}

// AnswerSources implements answers.Provider: "grafana alerts" in the smart
// search (ADR 0026).
func (m *Module) AnswerSources() []answers.Source {
	return []answers.Source{smartsearch.New(m.uc)}
}

// ConnectionStatus implements modules.StatusReporter (#599): the alert list
// the widget shows tells whether Grafana answers.
func (m *Module) ConnectionStatus(ctx context.Context) modules.ConnectionStatus {
	_, err := m.uc.Alerts(ctx)
	return modules.StatusOf(err, domain.ErrNotConfigured)
}

// Discovery implements modules.Discoverable (#627): Grafana answers its
// health check without a token.
func (m *Module) Discovery() modules.DiscoverySpec {
	return modules.DiscoverySpec{
		Service: "Grafana", Images: []string{"grafana"}, Hostnames: []string{"grafana", "grafana.local"},
		Ports: []int{3000}, ProbePath: "/api/health", ProbeContains: `"database"`,
	}
}
