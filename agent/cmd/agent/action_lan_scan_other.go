//go:build !windows

package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

var lanScanPorts = []int{22, 80, 135, 139, 443, 445, 515, 631, 3389, 5357, 8000, 8080, 9100}

func scanLANHosts(ctx context.Context, addresses []net.IP, _ net.IP) ([]lanScanHost, error) {
	targets := make(chan net.IP)
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
				if host, ok := scanLANHost(ctx, target); ok {
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
	hosts := make([]lanScanHost, 0, 32)
	for host := range results {
		hosts = append(hosts, host)
	}
	return hosts, ctx.Err()
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
	resolveLANScanHostName(ctx, &host)
	return host, true
}
