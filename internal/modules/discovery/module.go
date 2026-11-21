package discovery

import (
	"context"
	"net/http"
	"net/netip"

	"waylight/internal/modules"
	"waylight/internal/modules/discovery/adapter/controller"
	"waylight/internal/modules/discovery/adapter/gateway"
	"waylight/internal/modules/discovery/usecase"
	"waylight/internal/shared/kernel"
)

// Containers lists Docker containers (the docker module), nil when it is off.
type Containers func(ctx context.Context) ([]usecase.Container, error)

// Module finds services of the integrations in the home network (#590).
type Module struct {
	modules.Base
	handlers *controller.Handlers
}

// New collects the specs of the registered modules; build it after them.
// client must follow the integration URL rules; allowed is ALLOWED_CIDRS.
func New(all []modules.Module, containers Containers, client *http.Client, allowed []netip.Prefix, clock kernel.Clock) *Module {
	specs := map[string]usecase.Spec{}
	for _, m := range all {
		if d, ok := m.(modules.Discoverable); ok {
			s := d.Discovery()
			specs[m.Name()] = usecase.Spec{Service: s.Service, Images: s.Images, Hostnames: s.Hostnames,
				Ports: s.Ports, ProbePath: s.ProbePath, ProbeContains: s.ProbeContains, ServiceTypes: s.ServiceTypes}
		}
	}
	uc := usecase.New(usecase.Deps{
		Specs: specs, Containers: containers, Gateway: gateway.DefaultGateway, Resolve: gateway.Resolve,
		Browse: gateway.NewBrowser().Browse,
		Probe:  gateway.NewProber(client).Probe, Allowed: allowed, Clock: clock,
	})
	return &Module{handlers: controller.NewHandlers(uc)}
}

// Name implements modules.Module.
func (m *Module) Name() string { return "discovery" }

// DataKind says discovery is the instance's business; it stores nothing.
func (m *Module) DataKind() modules.DataKind { return modules.InstanceData }

// RegisterRoutes implements modules.Module.
func (m *Module) RegisterRoutes(r modules.Router) {
	r.Protected("GET /api/discovery/{integration}", m.handlers.Find)
}
