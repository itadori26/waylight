package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"waylight/internal/modules/discovery/usecase"
)

const (
	// mdnsGroup is the IPv4 multicast group and port of mDNS (RFC 6762).
	mdnsGroup = "224.0.0.251:5353"
	// maxMDNSMessage bounds one answer; mDNS messages fit a single packet.
	maxMDNSMessage = 9000
	// maxMDNSAnswers bounds the answers read per query, so a busy network
	// cannot keep a search busy.
	maxMDNSAnswers = 32
	// maxAnnouncements bounds the services returned per query.
	maxAnnouncements = 8
)

// Browser sends DNS-SD queries over mDNS (#627). It asks from an ephemeral
// port, a "legacy unicast" query (RFC 6762, section 6.7): responders answer
// that socket directly, so no listener on port 5353 is needed and nothing
// is joined or announced.
type Browser struct {
	// group is where queries go; tests point it at a fake responder.
	group string
}

// NewBrowser returns a browser for the local network.
func NewBrowser() *Browser { return &Browser{group: mdnsGroup} }

// Browse sends one PTR query for serviceType (e.g. "_home-assistant._tcp")
// and collects the answers until ctx ends. Only announcements with an
// address and a port are returned.
func (b *Browser) Browse(ctx context.Context, serviceType string) ([]usecase.Announcement, error) {
	name, err := dnsmessage.NewName(strings.TrimSuffix(serviceType, ".") + ".local.")
	if err != nil {
		return nil, fmt.Errorf("service type: %w", err)
	}
	query, err := packQuery(name)
	if err != nil {
		return nil, err
	}
	group, err := net.ResolveUDPAddr("udp4", b.group)
	if err != nil {
		return nil, fmt.Errorf("mdns group: %w", err)
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	_ = conn.SetDeadline(deadline)
	// Closing the socket ends the read loop when ctx is cancelled early.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := conn.WriteToUDP(query, group); err != nil {
		return nil, fmt.Errorf("send query: %w", err)
	}
	r := newRecords()
	buf := make([]byte, maxMDNSMessage)
	for range maxMDNSAnswers {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() || ctx.Err() != nil {
				break
			}
			return r.announcements(name), fmt.Errorf("read answer: %w", err)
		}
		r.add(buf[:n])
	}
	return r.announcements(name), nil
}

// packQuery builds a PTR question with the unicast-response bit set.
func packQuery(name dnsmessage.Name) ([]byte, error) {
	msg := dnsmessage.Message{Questions: []dnsmessage.Question{{
		Name: name, Type: dnsmessage.TypePTR,
		// The top bit of the class asks for a unicast answer (QU).
		Class: dnsmessage.ClassINET | 1<<15,
	}}}
	out, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack query: %w", err)
	}
	return out, nil
}

type srv struct {
	target string
	port   int
}

// records gathers what the answers say, from the answer and additional
// sections alike, since responders put SRV and address records in either.
type records struct {
	ptr   map[string][]string // service type -> instance names
	srv   map[string]srv      // instance -> host and port
	addrs map[string][]netip.Addr
}

func newRecords() *records {
	return &records{ptr: map[string][]string{}, srv: map[string]srv{}, addrs: map[string][]netip.Addr{}}
}

func (r *records) add(packet []byte) {
	var msg dnsmessage.Message
	if err := msg.Unpack(packet); err != nil || !msg.Response {
		return
	}
	for _, rr := range append(msg.Answers, msg.Additionals...) {
		key := strings.ToLower(rr.Header.Name.String())
		switch body := rr.Body.(type) {
		case *dnsmessage.PTRResource:
			r.ptr[key] = append(r.ptr[key], body.PTR.String())
		case *dnsmessage.SRVResource:
			r.srv[key] = srv{target: strings.ToLower(body.Target.String()), port: int(body.Port)}
		case *dnsmessage.AResource:
			r.addrs[key] = append(r.addrs[key], netip.AddrFrom4(body.A))
		case *dnsmessage.AAAAResource:
			r.addrs[key] = append(r.addrs[key], netip.AddrFrom16(body.AAAA))
		}
	}
}

// announcements joins PTR, SRV and address records of the queried type.
func (r *records) announcements(name dnsmessage.Name) []usecase.Announcement {
	var out []usecase.Announcement
	seen := map[string]bool{}
	for _, inst := range r.ptr[strings.ToLower(name.String())] {
		key := strings.ToLower(inst)
		s, ok := r.srv[key]
		if !ok || seen[key] || s.port == 0 || len(r.addrs[s.target]) == 0 {
			continue
		}
		seen[key] = true
		out = append(out, usecase.Announcement{
			Instance: instanceLabel(inst, name.String()),
			Host:     strings.TrimSuffix(s.target, "."),
			Addrs:    r.addrs[s.target],
			Port:     s.port,
		})
		if len(out) == maxAnnouncements {
			break
		}
	}
	return out
}

// instanceLabel is the readable part of an instance name, e.g. "Home" for
// "Home._home-assistant._tcp.local.". DNS escapes are left as they are.
func instanceLabel(instance, serviceType string) string {
	if len(instance) > len(serviceType) && strings.EqualFold(instance[len(instance)-len(serviceType):], serviceType) {
		instance = instance[:len(instance)-len(serviceType)]
	}
	return strings.TrimSuffix(instance, ".")
}
