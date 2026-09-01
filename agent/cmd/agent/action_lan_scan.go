package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

var lanScanPorts = []int{22, 80, 135, 139, 443, 445, 515, 631, 3389, 5357, 8000, 8080, 9100}

type lanScanHost struct {
	IP        string `json:"ip"`
	Name      string `json:"name,omitempty"`
	Latency   int64  `json:"latencyMs"`
	OpenPorts []int  `json:"openPorts"`
}

type lanScanResult struct {
	Subnet    string        `json:"subnet"`
	AgentIP   string        `json:"agentIp"`
	Scanned   int           `json:"scanned"`
	Hosts     []lanScanHost `json:"hosts"`
	StartedAt time.Time     `json:"startedAt"`
	Duration  int64         `json:"durationMs"`
}

func executeLANScan(ctx context.Context, requestedSubnet string) remoteJobResult {
	network, agentIP, err := resolveLANScanNetwork(requestedSubnet)
	if err != nil {
		return failedAction(err.Error())
	}
	addresses := usableIPv4Addresses(network)
	if len(addresses) == 0 || len(addresses) > 256 {
		return failedAction("в выбранной подсети нет допустимых адресов или превышен лимит 256")
	}
	started := time.Now().UTC()
	type scanTarget struct{ ip net.IP }
	targets := make(chan scanTarget)
	results := make(chan lanScanHost, len(addresses))
	workerCount := 32
	if len(addresses) < workerCount {
		workerCount = len(addresses)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range targets {
				if host, ok := scanLANHost(ctx, target.ip); ok {
					select {
					case results <- host:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(targets)
		for _, address := range addresses {
			select {
			case targets <- scanTarget{ip: address}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	hosts := make([]lanScanHost, 0, 32)
	for host := range results {
		hosts = append(hosts, host)
	}
	if err := ctx.Err(); err != nil {
		return failedAction("сканирование локальной сети прервано: " + err.Error())
	}
	sort.Slice(hosts, func(left, right int) bool {
		return bytesCompareIP(net.ParseIP(hosts[left].IP), net.ParseIP(hosts[right].IP)) < 0
	})
	result := lanScanResult{
		Subnet: network.String(), AgentIP: agentIP.String(), Scanned: len(addresses), Hosts: hosts,
		StartedAt: started, Duration: time.Since(started).Milliseconds(),
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return failedAction("не удалось сформировать результат сканирования")
	}
	return remoteJobResult{Success: true, Output: string(payload), ExitCode: 0}
}

func resolveLANScanNetwork(requested string) (*net.IPNet, net.IP, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		ip, network, err := net.ParseCIDR(requested)
		if err != nil || ip.To4() == nil || !ip.IsPrivate() {
			return nil, nil, errors.New("сканировать можно только внутреннюю IPv4-подсеть")
		}
		ones, bits := network.Mask.Size()
		if bits != 32 || ones < 24 || ones > 32 {
			return nil, nil, errors.New("подсеть должна содержать не более 256 адресов")
		}
		agentIP := localIPv4Inside(network)
		if agentIP == nil {
			agentIP = ip.To4()
		}
		return network, agentIP, nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось прочитать сетевые интерфейсы: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, network, parseErr := net.ParseCIDR(address.String())
			if parseErr != nil || ip.To4() == nil || !ip.IsPrivate() || ip.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := network.Mask.Size()
			if ones < 24 {
				ones = 24
			}
			mask := net.CIDRMask(ones, 32)
			return &net.IPNet{IP: ip.To4().Mask(mask), Mask: mask}, ip.To4(), nil
		}
	}
	return nil, nil, errors.New("Agent не нашёл активную внутреннюю IPv4-подсеть")
}

func localIPv4Inside(network *net.IPNet) net.IP {
	if network == nil {
		return nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil && ip.To4() != nil && network.Contains(ip.To4()) {
				return ip.To4()
			}
		}
	}
	return nil
}

func usableIPv4Addresses(network *net.IPNet) []net.IP {
	if network == nil {
		return nil
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 24 {
		return nil
	}
	base := network.IP.To4()
	if base == nil {
		return nil
	}
	count := 1 << (32 - ones)
	start, end := 0, count
	if count > 2 {
		start, end = 1, count-1
	}
	addresses := make([]net.IP, 0, end-start)
	value := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	for offset := start; offset < end; offset++ {
		current := value + uint32(offset)
		addresses = append(addresses, net.IPv4(byte(current>>24), byte(current>>16), byte(current>>8), byte(current)))
	}
	return addresses
}

func scanLANHost(ctx context.Context, ip net.IP) (lanScanHost, bool) {
	started := time.Now()
	openPorts := make([]int, 0, 4)
	dialer := net.Dialer{Timeout: 140 * time.Millisecond}
	for _, port := range lanScanPorts {
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), fmt.Sprint(port)))
		if err == nil {
			openPorts = append(openPorts, port)
			_ = connection.Close()
		}
		if ctx.Err() != nil {
			return lanScanHost{}, false
		}
	}
	if len(openPorts) == 0 {
		return lanScanHost{}, false
	}
	host := lanScanHost{IP: ip.String(), Latency: max(time.Since(started).Milliseconds(), 1), OpenPorts: openPorts}
	lookupCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip.String()); err == nil && len(names) > 0 {
		host.Name = strings.TrimSuffix(names[0], ".")
	}
	return host, true
}

func bytesCompareIP(left, right net.IP) int {
	left, right = left.To4(), right.To4()
	for index := 0; index < 4; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}
