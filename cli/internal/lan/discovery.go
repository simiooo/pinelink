package lan

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/libp2p/zeroconf/v2"
)

type Nearby struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

func DefaultAddress(listener net.Listener) (string, error) {
	bound, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("local listener must use TCP")
	}
	ip := bound.IP
	if ip == nil || ip.IsUnspecified() {
		interfaces, err := net.Interfaces()
		if err != nil {
			return "", err
		}
		var private, other []net.IP
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addresses, _ := iface.Addrs()
			for _, address := range addresses {
				candidate, _, err := net.ParseCIDR(address.String())
				if err != nil || candidate.IsLinkLocalUnicast() || candidate.IsLoopback() {
					continue
				}
				if candidate.IsPrivate() && candidate.To4() != nil {
					private = append(private, candidate)
				} else if candidate.IsGlobalUnicast() {
					other = append(other, candidate)
				}
			}
		}
		if len(private) > 0 {
			ip = private[0]
		} else if len(other) > 0 {
			ip = other[0]
		} else {
			ip = net.ParseIP("127.0.0.1")
		}
	}
	return (&url.URL{Scheme: "https", Host: net.JoinHostPort(ip.String(), strconv.Itoa(bound.Port))}).String(), nil
}

func Advertise(peer Peer) (func(), error) {
	u, err := endpoint(peer.Address)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	if strings.Contains(host, "%") {
		host = strings.SplitN(host, "%", 2)[0]
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() {
		return nil, fmt.Errorf("loopback endpoints are not advertised")
	}
	port, _ := strconv.Atoi(u.Port())
	server, err := zeroconf.RegisterProxy(peer.ID[:16], ServiceType, "local.", port, peer.ID[:16]+".local.", []string{ip.String()}, []string{"v=1", "id=" + peer.ID, "name=" + peer.Name}, nil)
	if err != nil {
		return nil, err
	}
	return server.Shutdown, nil
}

func Discover(ctx context.Context) ([]Nearby, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	found := map[string]*Nearby{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstError error
	started := 0
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		started++
		iface := iface
		entries := make(chan *zeroconf.ServiceEntry, 16)
		wg.Go(func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			readerDone := make(chan struct{})
			go func() {
				defer close(readerDone)
				for {
					select {
					case <-ctx.Done():
						return
					case entry, ok := <-entries:
						if !ok {
							return
						}
						if entry == nil || entry.Port < 1 || entry.Port > 65535 {
							continue
						}
						var id, name, version string
						for _, t := range entry.Text {
							if strings.HasPrefix(t, "id=") {
								id = strings.TrimPrefix(t, "id=")
							}
							if strings.HasPrefix(t, "name=") {
								name = strings.TrimPrefix(t, "name=")
							}
							if strings.HasPrefix(t, "v=") {
								version = strings.TrimPrefix(t, "v=")
							}
						}
						_, invalidID := hex.DecodeString(id)
						if len(id) != 64 || invalidID != nil || id != strings.ToLower(id) || version != "1" || len(name) > 128 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
							continue
						}
						var addresses []string
						for _, ip := range append(entry.AddrIPv4, entry.AddrIPv6...) {
							host := ip.String()
							if ip.IsLinkLocalUnicast() && ip.To4() == nil {
								host += "%" + iface.Name
							}
							addresses = append(addresses, (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, strconv.Itoa(entry.Port))}).String())
						}
						mu.Lock()
						n := found[id]
						if n == nil {
							if len(found) >= 256 {
								mu.Unlock()
								continue
							}
							n = &Nearby{ID: id, Name: name}
							found[id] = n
						}
						for _, address := range addresses {
							seen := false
							for _, known := range n.Addresses {
								if known == address {
									seen = true
								}
							}
							if !seen {
								n.Addresses = append(n.Addresses, address)
							}
						}
						mu.Unlock()
					}
				}
			}()
			err := zeroconf.Browse(ctx, ServiceType, "local.", entries, zeroconf.SelectIfaces([]net.Interface{iface}))
			if err != nil && ctx.Err() == nil {
				mu.Lock()
				if firstError == nil {
					firstError = err
				}
				mu.Unlock()
			}
			// A socket setup error may return before Browse owns the channel.
			cancel()
			<-readerDone
		})
	}
	if started == 0 {
		return []Nearby{}, fmt.Errorf("local multicast discovery is unavailable; use a connection code")
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	list := make([]Nearby, 0, len(found))
	for _, v := range found {
		sort.Strings(v.Addresses)
		list = append(list, *v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	if len(list) == 0 && firstError != nil {
		return list, firstError
	}
	return list, nil
}
