//go:build windows

package main

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Windows discovery combines ICMP, the operating-system neighbour cache and a
// fixed allow-list of administration ports. The bounded probes find RDP/SSH and
// printers which intentionally ignore ping without turning Agent into an
// arbitrary port scanner.
func scanLANHosts(ctx context.Context, addresses []net.IP, agentIP net.IP) ([]lanScanHost, error) {
	allowed := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		allowed[address.String()] = struct{}{}
	}
	targets := make(chan net.IP)
	results := make(chan lanScanHost, len(addresses))
	workerCount := min(48, len(addresses))
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range targets {
				host, pingOK := pingWindowsLANHost(ctx, address)
				host.OpenPorts = probeKnownLANPorts(ctx, address.String())
				if pingOK || len(host.OpenPorts) > 0 {
					if host.IP == "" {
						host.IP = address.String()
					}
					resolveLANScanHostName(ctx, &host)
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
			case targets <- address:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()

	byIP := make(map[string]lanScanHost)
	for host := range results {
		byIP[host.IP] = host
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	neighbors, err := windowsLANNeighbors(ctx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		// ICMP results are still useful when the neighbor table is unavailable.
		neighbors = nil
	}
	for _, ip := range neighbors {
		if _, ok := allowed[ip.String()]; !ok {
			continue
		}
		if _, exists := byIP[ip.String()]; !exists {
			byIP[ip.String()] = lanScanHost{IP: ip.String(), OpenPorts: []int{}}
		}
	}
	if agentIP != nil {
		if _, ok := allowed[agentIP.String()]; ok {
			if _, exists := byIP[agentIP.String()]; !exists {
				byIP[agentIP.String()] = lanScanHost{IP: agentIP.String(), OpenPorts: []int{}}
			}
		}
	}
	hosts := make([]lanScanHost, 0, len(byIP))
	for _, host := range byIP {
		hosts = append(hosts, host)
	}
	resolveMissingWindowsLANHostNames(ctx, hosts)
	sort.Slice(hosts, func(left, right int) bool {
		return bytesCompareIP(net.ParseIP(hosts[left].IP), net.ParseIP(hosts[right].IP)) < 0
	})
	return hosts, nil
}

// Neighbours learned from ARP and the local Agent address are merged after the
// active probe workers finish. Enrich those rows as well, otherwise the scan
// shows an IP even when Windows can resolve a useful computer name. Reverse DNS
// is attempted for every unnamed host; NetBIOS is only used for Windows-like
// endpoints and is tightly bounded so discovery cannot stall on silent hosts.
func resolveMissingWindowsLANHostNames(ctx context.Context, hosts []lanScanHost) {
	if len(hosts) == 0 {
		return
	}
	indexes := make(chan int)
	workerCount := min(12, len(hosts))
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range indexes {
				if hosts[index].Name != "" {
					continue
				}
				resolveLANScanHostName(ctx, &hosts[index])
				if hosts[index].Name == "" && hasLANPort(hosts[index].OpenPorts, 445, 3389) {
					hosts[index].Name = windowsNetBIOSHostName(ctx, hosts[index].IP)
				}
			}
		}()
	}
	for index := range hosts {
		select {
		case indexes <- index:
		case <-ctx.Done():
			close(indexes)
			workers.Wait()
			return
		}
	}
	close(indexes)
	workers.Wait()
}

func hasLANPort(ports []int, expected ...int) bool {
	for _, port := range ports {
		for _, candidate := range expected {
			if port == candidate {
				return true
			}
		}
	}
	return false
}

func windowsNetBIOSHostName(ctx context.Context, ip string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 650*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(lookupCtx, "nbtstat.exe", "-A", ip)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return parseWindowsNetBIOSHostName(output)
}

func parseWindowsNetBIOSHostName(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		for index, field := range fields {
			if field != "<00>" || index == 0 {
				continue
			}
			name := strings.TrimSpace(fields[index-1])
			if name != "" && len(name) <= 63 && !strings.ContainsAny(name, "\\/:") {
				return name
			}
		}
	}
	return ""
}

func probeKnownLANPorts(ctx context.Context, host string) []int {
	// This is a bounded diagnostic probe, not an arbitrary port scanner. Only
	// well-known administration, printing and remote-access ports are checked.
	ports := []int{22, 80, 443, 445, 515, 631, 8291, 8728, 8729, 9100, 3389}
	open := make([]int, 0, 4)
	for _, port := range ports {
		dialer := net.Dialer{Timeout: 180 * time.Millisecond}
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			_ = connection.Close()
			open = append(open, port)
		}
	}
	return open
}

func pingWindowsLANHost(ctx context.Context, ip net.IP) (lanScanHost, bool) {
	started := time.Now()
	command := exec.CommandContext(ctx, "ping.exe", "-n", "1", "-w", "220", ip.String())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Run(); err != nil {
		return lanScanHost{}, false
	}
	host := lanScanHost{IP: ip.String(), Latency: max(time.Since(started).Milliseconds(), 1), OpenPorts: []int{}}
	return host, true
}

func windowsLANNeighbors(ctx context.Context) ([]net.IP, error) {
	command := exec.CommandContext(ctx, "arp.exe", "-a")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	return parseLANNeighborTable(output), nil
}
