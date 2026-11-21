package gateway

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// fakeResponder answers the first query like Home Assistant does: the PTR
// record in the answers, SRV and A records as additionals.
func fakeResponder(t *testing.T) (string, <-chan dnsmessage.Question) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	questions := make(chan dnsmessage.Question, 1)
	go func() {
		buf := make([]byte, 1500)
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var q dnsmessage.Message
		if q.Unpack(buf[:n]) != nil || len(q.Questions) != 1 {
			return
		}
		questions <- q.Questions[0]
		typ := dnsmessage.MustNewName("_home-assistant._tcp.local.")
		inst := dnsmessage.MustNewName("Home._home-assistant._tcp.local.")
		host := dnsmessage.MustNewName("homeassistant.local.")
		h := func(n dnsmessage.Name, ty dnsmessage.Type) dnsmessage.ResourceHeader {
			return dnsmessage.ResourceHeader{Name: n, Type: ty, Class: dnsmessage.ClassINET, TTL: 120}
		}
		answer := dnsmessage.Message{
			Header:  dnsmessage.Header{Response: true, Authoritative: true},
			Answers: []dnsmessage.Resource{{Header: h(typ, dnsmessage.TypePTR), Body: &dnsmessage.PTRResource{PTR: inst}}},
			Additionals: []dnsmessage.Resource{
				{Header: h(inst, dnsmessage.TypeSRV), Body: &dnsmessage.SRVResource{Target: host, Port: 8123}},
				{Header: h(host, dnsmessage.TypeA), Body: &dnsmessage.AResource{A: [4]byte{192, 168, 178, 30}}},
			},
		}
		out, _ := answer.Pack()
		_, _ = conn.WriteToUDP(out, from)
	}()
	return conn.LocalAddr().String(), questions
}

func TestBrowse(t *testing.T) {
	addr, questions := fakeResponder(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	list, err := (&Browser{group: addr}).Browse(ctx, "_home-assistant._tcp")
	if err != nil {
		t.Fatal(err)
	}
	q := <-questions
	if q.Type != dnsmessage.TypePTR || q.Name.String() != "_home-assistant._tcp.local." || q.Class&(1<<15) == 0 {
		t.Fatalf("query: %+v", q)
	}
	if len(list) != 1 || list[0].Instance != "Home" || list[0].Port != 8123 || list[0].Host != "homeassistant.local" ||
		len(list[0].Addrs) != 1 || list[0].Addrs[0] != netip.MustParseAddr("192.168.178.30") {
		t.Fatalf("announcements: %+v", list)
	}
}

// TestRecordsNeedAddressAndPort drops an instance whose SRV or address
// record is missing: there is nothing to probe.
func TestRecordsNeedAddressAndPort(t *testing.T) {
	r := newRecords()
	r.ptr["_x._tcp.local."] = []string{"a._x._tcp.local."}
	r.srv["a._x._tcp.local."] = srv{target: "a.local.", port: 80}
	if got := r.announcements(dnsmessage.MustNewName("_x._tcp.local.")); len(got) != 0 {
		t.Fatalf("without an address: %+v", got)
	}
}
