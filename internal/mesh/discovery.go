package mesh

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	MulticastAddr  = "239.42.42.42:4242"
	BeaconMagic    = "ISKRA1"
	BeaconInterval = 5 * time.Second // Aggressive for alpha — find peers fast
	BeaconSize     = 6 + 32 + 2 + 1 + 8 // magic + pubkey + port + version + timestamp = 49 bytes
)

// Discovery handles LAN multicast peer discovery.
type Discovery struct {
	pubKey     [32]byte
	listenPort uint16
	version    uint8
	peers      *PeerList
	onPeer     func(pubKey [32]byte, ip string, port uint16)
	stop       chan struct{}
}

// NewDiscovery creates a new LAN discovery service.
func NewDiscovery(pubKey [32]byte, listenPort uint16, peers *PeerList) *Discovery {
	return &Discovery{
		pubKey:     pubKey,
		listenPort: listenPort,
		version:    1,
		peers:      peers,
		stop:       make(chan struct{}),
	}
}

// SetOnPeer sets a callback for when a new peer is discovered.
func (d *Discovery) SetOnPeer(fn func(pubKey [32]byte, ip string, port uint16)) {
	d.onPeer = fn
}

// Start begins sending and listening for beacons on ALL network interfaces.
func (d *Discovery) Start() error {
	// Start listeners on all interfaces
	ifaces := multicastInterfaces()
	if len(ifaces) == 0 {
		log.Printf("[Discovery] No multicast interfaces found, starting on default")
		go d.listenOnInterface(nil)
		go d.sendOnInterface(nil)
	} else {
		for _, iface := range ifaces {
			iface := iface // capture
			log.Printf("[Discovery] Starting on interface %s (%s)", iface.Name, ifaceAddrs(&iface))
			go d.listenOnInterface(&iface)
			go d.sendOnInterface(&iface)
		}
	}
	return nil
}

// Stop stops discovery.
func (d *Discovery) Stop() {
	close(d.stop)
}

// multicastInterfaces returns all active interfaces that support multicast.
func multicastInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var result []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Must have at least one IPv4 address
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		hasIPv4 := false
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				hasIPv4 = true
				break
			}
		}
		if hasIPv4 {
			result = append(result, iface)
		}
	}
	return result
}

// ifaceAddrs returns a summary of interface addresses for logging.
func ifaceAddrs(iface *net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		return "no addrs"
	}
	s := ""
	for _, a := range addrs {
		if s != "" {
			s += ", "
		}
		s += a.String()
	}
	return s
}

// makeBeacon creates the beacon packet.
func (d *Discovery) makeBeacon() []byte {
	buf := make([]byte, BeaconSize)
	copy(buf[0:6], BeaconMagic)
	copy(buf[6:38], d.pubKey[:])
	binary.BigEndian.PutUint16(buf[38:40], d.listenPort)
	buf[40] = d.version
	binary.BigEndian.PutUint64(buf[41:49], uint64(time.Now().Unix()))
	return buf
}

// parseBeacon parses a beacon packet.
func parseBeacon(data []byte) (pubKey [32]byte, port uint16, version uint8, err error) {
	if len(data) < BeaconSize {
		return pubKey, 0, 0, fmt.Errorf("beacon too short: %d bytes", len(data))
	}
	if string(data[0:6]) != BeaconMagic {
		return pubKey, 0, 0, fmt.Errorf("invalid beacon magic")
	}
	copy(pubKey[:], data[6:38])
	port = binary.BigEndian.Uint16(data[38:40])
	version = data[40]
	return pubKey, port, version, nil
}

func (d *Discovery) sendOnInterface(iface *net.Interface) {
	addr, err := net.ResolveUDPAddr("udp4", MulticastAddr)
	if err != nil {
		return
	}

	// Bind to specific interface by using its local address
	var localAddr *net.UDPAddr
	if iface != nil {
		addrs, err := iface.Addrs()
		if err == nil {
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
					localAddr = &net.UDPAddr{IP: ipnet.IP}
					break
				}
			}
		}
	}

	conn, err := net.DialUDP("udp4", localAddr, addr)
	if err != nil {
		ifName := "default"
		if iface != nil {
			ifName = iface.Name
		}
		log.Printf("[Discovery] Failed to dial multicast on %s: %v", ifName, err)
		return
	}
	defer conn.Close()

	ifName := "default"
	if iface != nil {
		ifName = iface.Name
	}
	log.Printf("[Discovery] Sending beacons on %s to %s (port %d)", ifName, MulticastAddr, d.listenPort)
	conn.Write(d.makeBeacon())

	ticker := time.NewTicker(BeaconInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			conn.Write(d.makeBeacon())
		}
	}
}

func (d *Discovery) listenOnInterface(iface *net.Interface) {
	addr, err := net.ResolveUDPAddr("udp4", MulticastAddr)
	if err != nil {
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		ifName := "default"
		if iface != nil {
			ifName = iface.Name
		}
		log.Printf("[Discovery] Failed to listen multicast on %s: %v", ifName, err)
		return
	}
	defer conn.Close()

	ifName := "default"
	if iface != nil {
		ifName = iface.Name
	}
	log.Printf("[Discovery] Listening for beacons on %s (%s)", ifName, MulticastAddr)

	conn.SetReadBuffer(1024)

	buf := make([]byte, 1024)
	for {
		select {
		case <-d.stop:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		pubKey, port, _, err := parseBeacon(buf[:n])
		if err != nil {
			continue
		}

		// Ignore our own beacon
		if pubKey == d.pubKey {
			continue
		}

		ip := src.IP.String()
		log.Printf("[Discovery] Found peer %s:%d via %s", ip, port, ifName)
		d.peers.AddOrUpdate(pubKey, ip, port)

		if d.onPeer != nil {
			d.onPeer(pubKey, ip, port)
		}
	}
}

// SendBeaconTo sends a beacon directly to a specific address (for testing).
func (d *Discovery) SendBeaconTo(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(d.makeBeacon())
	return err
}
