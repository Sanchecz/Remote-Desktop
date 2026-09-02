package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

type lanScanHost struct {
	IP        string `json:"ip"`
	Name      string `json:"name,omitempty"`
	Latency   int64  `json:"latencyMs"`
	OpenPorts []int  `json:"openPorts"`
}

type lanScanResult struct {
	Subnet           string             `json:"subnet"`
	AgentIP          string             `json:"agentIp"`
	AvailableSubnets []lanScanCandidate `json:"availableSubnets"`
	Scanned          int                `json:"scanned"`
	Hosts            []lanScanHost      `json:"hosts"`
	StartedAt        time.Time          `json:"startedAt"`
	Duration         int64              `json:"durationMs"`
}

type lanScanCandidate struct {
	Subnet    string `json:"subnet"`
	AgentIP   string `json:"agentIp"`
	Interface string `json:"interface"`
	Preferred bool   `json:"preferred"`
	score     int
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
	hosts, scanErr := scanLANHosts(ctx, addresses, agentIP)
	if scanErr != nil {
		return failedAction("сканирование локальной сети не выполнено: " + scanErr.Error())
	}
	if err := ctx.Err(); err != nil {
		return failedAction("сканирование локальной сети прервано: " + err.Error())
	}
	sort.Slice(hosts, func(left, right int) bool {
		return bytesCompareIP(net.ParseIP(hosts[left].IP), net.ParseIP(hosts[right].IP)) < 0
	})
	candidates, _ := listLANScanCandidates()
	result := lanScanResult{
		Subnet: network.String(), AgentIP: agentIP.String(), AvailableSubnets: candidates, Scanned: len(addresses), Hosts: hosts,
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
	candidates, err := listLANScanCandidates()
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось прочитать сетевые интерфейсы: %w", err)
	}
	if len(candidates) > 0 {
		_, network, parseErr := net.ParseCIDR(candidates[0].Subnet)
		if parseErr == nil {
			return network, net.ParseIP(candidates[0].AgentIP).To4(), nil
		}
	}
	return nil, nil, errors.New("Agent не нашёл активную внутреннюю IPv4-подсеть")
}

func listLANScanCandidates() ([]lanScanCandidate, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	bySubnet := make(map[string]lanScanCandidate)
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
			subnet := (&net.IPNet{IP: ip.To4().Mask(mask), Mask: mask}).String()
			candidate := lanScanCandidate{Subnet: subnet, AgentIP: ip.To4().String(), Interface: iface.Name, score: lanInterfaceScore(iface.Name, ip.To4(), ones)}
			if previous, exists := bySubnet[subnet]; !exists || candidate.score > previous.score {
				bySubnet[subnet] = candidate
			}
		}
	}
	candidates := make([]lanScanCandidate, 0, len(bySubnet))
	for _, candidate := range bySubnet {
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].score != candidates[right].score {
			return candidates[left].score > candidates[right].score
		}
		return candidates[left].Subnet < candidates[right].Subnet
	})
	if len(candidates) > 0 {
		candidates[0].Preferred = true
	}
	return candidates, nil
}

func lanInterfaceScore(name string, ip net.IP, prefix int) int {
	value := strings.ToLower(name)
	score := 0
	for _, physical := range []string{"ethernet", "wi-fi", "wifi", "wlan", "беспровод", "локальная сеть"} {
		if strings.Contains(value, physical) {
			score += 100
			break
		}
	}
	for _, virtual := range []string{"virtual", "vethernet", "hyper-v", "wireguard", "wintun", "vpn", "tunnel", "tailscale", "zerotier", "docker", "wsl", "loopback"} {
		if strings.Contains(value, virtual) {
			score -= 180
			break
		}
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 192 && ip4[1] == 168 {
			score += 25
		} else if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			score += 15
		}
	}
	if prefix == 24 {
		score += 5
	}
	return score
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

func resolveLANScanHostName(ctx context.Context, host *lanScanHost) {
	if host == nil || net.ParseIP(host.IP) == nil {
		return
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if names, err := net.DefaultResolver.LookupAddr(lookupCtx, host.IP); err == nil && len(names) > 0 {
		host.Name = strings.TrimSuffix(names[0], ".")
	}
}

func parseLANNeighborTable(output []byte) []net.IP {
	seen := make(map[string]struct{})
	neighbors := make([]net.IP, 0, 32)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(fields[0]).To4()
		if ip == nil || !ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		if _, ok := seen[ip.String()]; ok {
			continue
		}
		seen[ip.String()] = struct{}{}
		neighbors = append(neighbors, ip)
	}
	return neighbors
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
