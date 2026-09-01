//go:build windows

package main

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Windows discovery intentionally uses ICMP and the operating system neighbor
// cache instead of probing a list of TCP ports. It finds devices on the LAN for
// the administrator without making the long-lived remote-control binary look
// or behave like a generic port scanner.
func scanLANHosts(ctx context.Context, addresses []net.IP, agentIP net.IP) ([]lanScanHost, error) {
	allowed := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		allowed[address.String()] = struct{}{}
	}
	targets := make(chan net.IP)
	results := make(chan lanScanHost, len(addresses))
	workerCount := min(16, len(addresses))
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range targets {
				if host, ok := pingWindowsLANHost(ctx, address); ok {
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
	sort.Slice(hosts, func(left, right int) bool {
		return bytesCompareIP(net.ParseIP(hosts[left].IP), net.ParseIP(hosts[right].IP)) < 0
	})
	return hosts, nil
}

func pingWindowsLANHost(ctx context.Context, ip net.IP) (lanScanHost, bool) {
	started := time.Now()
	command := exec.CommandContext(ctx, "ping.exe", "-n", "1", "-w", "220", ip.String())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Run(); err != nil {
		return lanScanHost{}, false
	}
	host := lanScanHost{IP: ip.String(), Latency: max(time.Since(started).Milliseconds(), 1), OpenPorts: []int{}}
	resolveLANScanHostName(ctx, &host)
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
