package remote

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestDiscoveryNetworks(t *testing.T) {
	var addresses []netip.Addr
	for _, raw := range []string{"10.2.3.4", "10.9.8.7", "172.20.1.2", "192.168.5.2", "127.0.0.1", "169.254.1.2", "8.8.8.8", "fd00::1", "::ffff:10.1.1.1"} {
		addresses = append(addresses, netip.MustParseAddr(raw))
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16")}
	if got := DiscoveryNetworks(addresses); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestHosts(t *testing.T) {
	if got := Hosts([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.168.0.0/16")}); got != 1<<24+1<<16 {
		t.Fatalf("hosts: %d", got)
	}
}
