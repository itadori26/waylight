package modules

// DiscoverySpec says, as data, how an integration is found in the home
// network for its widget setup (#590). Every candidate is confirmed with
// the probe before it is offered, so only real matches appear.
type DiscoverySpec struct {
	// Service is the product name in its own spelling, e.g. "Immich".
	Service string
	// Images are lower-case parts of Docker image names, e.g. "immich-server".
	Images []string
	// Hostnames are names the service usually answers to in a home
	// network, e.g. "homeassistant.local".
	Hostnames []string
	// Ports are the service's default ports; only these are tried, never a
	// range.
	Ports []int
	// ProbePath is requested without credentials, and ProbeContains must
	// appear in the answer, e.g. "/api/server/ping" and "pong".
	ProbePath, ProbeContains string
	// ServiceTypes are DNS-SD service types the service announces over
	// mDNS, e.g. "_home-assistant._tcp" (#627). One query per type is sent
	// per search; the answers go through the same address rules and probe.
	ServiceTypes []string
}

// Discoverable is implemented by modules whose service can be found in the
// home network.
type Discoverable interface {
	Discovery() DiscoverySpec
}
