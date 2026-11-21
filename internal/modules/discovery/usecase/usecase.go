package usecase

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"waylight/internal/modules/discovery/domain"
	"waylight/internal/shared/kernel"
)

// Spec is how one integration is found (modules.DiscoverySpec).
type Spec struct {
	Service                  string
	Images, Hostnames        []string
	Ports                    []int
	ProbePath, ProbeContains string
	// ServiceTypes are DNS-SD types such as "_home-assistant._tcp" (#627).
	ServiceTypes []string
}

// Announcement is a service that answered a DNS-SD query over mDNS.
type Announcement struct {
	// Instance is the name the service announces, e.g. "Home".
	Instance string
	Host     string
	Addrs    []netip.Addr
	Port     int
}

// Container is a Docker container with its exposed TCP ports and addresses.
type Container struct {
	Name, Image string
	// Ports maps private to public ports; public is 0 when not published.
	Ports     []struct{ Private, Public int }
	Addresses []netip.Addr
}

// Deps bundles the output ports. Containers, Gateway, Resolve and Browse
// may be nil.
type Deps struct {
	Specs map[string]Spec
	// Containers lists the containers when the docker module is on.
	Containers func(ctx context.Context) ([]Container, error)
	// Gateway is the default gateway: the router, or the Docker host when
	// Waylight runs in a container.
	Gateway func() (netip.Addr, bool)
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// Browse sends one DNS-SD query for a service type on the local
	// network and returns the answers until ctx ends (#627).
	Browse func(ctx context.Context, serviceType string) ([]Announcement, error)
	// Probe requests baseURL+path without credentials and reports whether
	// the answer contains the marker.
	Probe   func(ctx context.Context, baseURL, path, contains string) bool
	Allowed []netip.Prefix
	Clock   kernel.Clock
}

// Discovery is the input port of the module (#590).
type Discovery interface {
	// Find lists the confirmed services of an integration; admins only.
	Find(ctx context.Context, integration string) ([]domain.Found, error)
}

// ErrUnknown is returned for an integration that cannot be found this way.
var ErrUnknown = kernel.NewPublicError(kernel.ErrNotFound, "This integration cannot be found in the home network.")

const (
	// cacheTTL keeps results for a few minutes, so reopening a setup does
	// not probe again; they are never stored.
	cacheTTL = 3 * time.Minute
	// probeTimeout bounds one probe, searchTimeout the whole search.
	probeTimeout  = 1500 * time.Millisecond
	searchTimeout = 6 * time.Second
	// probeWorkers bounds the probes in flight.
	probeWorkers = 4
	// browseTimeout is how long answers to one mDNS query are collected;
	// responders answer within a few hundred milliseconds.
	browseTimeout = 1200 * time.Millisecond
	// maxServiceTypes bounds the mDNS queries one search sends.
	maxServiceTypes = 3
)

type entry struct {
	found []domain.Found
	at    time.Time
}

type discovery struct {
	d     Deps
	mu    sync.Mutex
	cache map[string]entry
}

// New returns the use case.
func New(d Deps) Discovery { return &discovery{d: d, cache: map[string]entry{}} }

func (u *discovery) Find(ctx context.Context, integration string) ([]domain.Found, error) {
	if !kernel.PrincipalFrom(ctx).IsAdmin() {
		return nil, kernel.ErrForbidden
	}
	spec, ok := u.d.Specs[integration]
	if !ok {
		return nil, ErrUnknown
	}
	now := u.d.Clock.Now()
	u.mu.Lock()
	e, hit := u.cache[integration]
	u.mu.Unlock()
	if hit && now.Sub(e.at) < cacheTTL {
		return e.found, nil
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	plan := domain.Plan(u.candidates(ctx, spec), u.d.Allowed)
	found := u.probe(ctx, spec, plan)
	u.mu.Lock()
	u.cache[integration] = entry{found: found, at: now}
	u.mu.Unlock()
	return found, nil
}

// candidates collects addresses in order of confidence: containers of the
// service's image, services that announce themselves over mDNS, the usual
// host names, then the default ports on the gateway.
func (u *discovery) candidates(ctx context.Context, spec Spec) []domain.Candidate {
	var out []domain.Candidate
	gw, haveGW := netip.Addr{}, false
	if u.d.Gateway != nil {
		gw, haveGW = u.d.Gateway()
	}
	if u.d.Containers != nil {
		if list, err := u.d.Containers(ctx); err == nil {
			for _, c := range list {
				if matchesImage(c.Image, spec.Images) {
					out = append(out, fromContainer(c, spec, gw, haveGW)...)
				}
			}
		}
	}
	out = append(out, u.announced(ctx, spec)...)
	if u.d.Resolve != nil {
		for _, host := range spec.Hostnames {
			addrs, err := u.d.Resolve(ctx, host)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				for _, p := range spec.Ports {
					out = append(out, domain.Candidate{Addr: a, Port: p, Host: host, Hint: host})
				}
			}
		}
	}
	if haveGW {
		for _, p := range spec.Ports {
			out = append(out, domain.Candidate{Addr: gw, Port: p, Hint: "gateway " + gw.String()})
		}
	}
	return out
}

// announced asks once per service type who announces it; the answers
// carry the port, so each address is tried on that port only.
func (u *discovery) announced(ctx context.Context, spec Spec) []domain.Candidate {
	if u.d.Browse == nil {
		return nil
	}
	var out []domain.Candidate
	for i, t := range spec.ServiceTypes {
		if i == maxServiceTypes {
			break
		}
		bctx, cancel := context.WithTimeout(ctx, browseTimeout)
		list, err := u.d.Browse(bctx, t)
		cancel()
		if err != nil && len(list) == 0 {
			continue
		}
		for _, a := range list {
			hint := "mDNS " + a.Instance
			if a.Instance == "" {
				hint = "mDNS " + a.Host
			}
			for _, addr := range a.Addrs {
				out = append(out, domain.Candidate{Addr: addr, Port: a.Port, Hint: hint})
			}
		}
	}
	return out
}

func matchesImage(image string, parts []string) bool {
	image = strings.ToLower(image)
	for _, p := range parts {
		if strings.Contains(image, p) {
			return true
		}
	}
	return false
}

// fromContainer tries the container's own addresses (a shared Docker
// network) and its published ports on the gateway (the Docker host).
func fromContainer(c Container, spec Spec, gw netip.Addr, haveGW bool) []domain.Candidate {
	hint := "Docker container " + c.Name
	var out []domain.Candidate
	ports := c.Ports
	if len(ports) == 0 {
		for _, p := range spec.Ports {
			ports = append(ports, struct{ Private, Public int }{Private: p})
		}
	}
	for _, p := range ports {
		for _, a := range c.Addresses {
			out = append(out, domain.Candidate{Addr: a, Port: p.Private, Hint: hint})
		}
		if p.Public > 0 && haveGW {
			out = append(out, domain.Candidate{Addr: gw, Port: p.Public, Hint: hint})
		}
	}
	return out
}

func (u *discovery) probe(ctx context.Context, spec Spec, plan []domain.Candidate) []domain.Found {
	ok := make([]bool, len(plan))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range probeWorkers {
		wg.Go(func() {
			for i := range jobs {
				pctx, cancel := context.WithTimeout(ctx, probeTimeout)
				ok[i] = u.d.Probe(pctx, plan[i].URL(), spec.ProbePath, spec.ProbeContains)
				cancel()
			}
		})
	}
	for i := range plan {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	out := []domain.Found{}
	for i, c := range plan {
		if ok[i] {
			out = append(out, domain.Found{URL: c.URL(), Hint: c.Hint})
		}
	}
	return out
}
