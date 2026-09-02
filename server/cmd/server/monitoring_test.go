package main

import (
	"slices"
	"testing"
	"time"
)

func healthyMonitoredDevice(now time.Time) monitoredDevice {
	return monitoredDevice{
		ID: "device-1", Name: "Workstation", OS: "windows", AgentVersion: monitoredAgentVersion, InstallMode: "system",
		CPULoad: 18, MemoryBytes: 16 << 30, MemoryUsed: 7 << 30, DiskTotal: 512 << 30, DiskFree: 240 << 30,
		Privileged: true, LastSeen: now.Add(-15 * time.Second),
	}
}

func TestEvaluateMonitoredDeviceHealthy(t *testing.T) {
	now := time.Now()
	snapshot := evaluateMonitoredDevice(healthyMonitoredDevice(now), now)
	if snapshot.Status != "ok" || len(snapshot.ProblemCodes) != 0 {
		t.Fatalf("healthy device was classified as %#v", snapshot)
	}
}

func TestEvaluateMonitoredDeviceResourceThresholds(t *testing.T) {
	now := time.Now()
	device := healthyMonitoredDevice(now)
	device.CPULoad = 88
	device.MemoryUsed = 15.4 * (1 << 30)
	device.DiskFree = 20 * (1 << 30)
	snapshot := evaluateMonitoredDevice(device, now)
	if snapshot.Status != "down" {
		t.Fatalf("critical resources must be down, got %q", snapshot.Status)
	}
	for _, expected := range []string{"cpu_high", "memory_critical", "disk_critical"} {
		if !slices.Contains(snapshot.ProblemCodes, expected) {
			t.Fatalf("missing %q in %#v", expected, snapshot.ProblemCodes)
		}
	}
}

func TestEvaluateMonitoredDeviceOfflineSuppressesStaleMetrics(t *testing.T) {
	now := time.Now()
	device := healthyMonitoredDevice(now)
	device.LastSeen = now.Add(-2 * time.Minute)
	device.CPULoad = 100
	device.RemoteError = "stale capture failure"
	snapshot := evaluateMonitoredDevice(device, now)
	if snapshot.Status != "down" || !slices.Equal(snapshot.ProblemCodes, []string{"agent_offline"}) {
		t.Fatalf("offline device must report only the current connection fault, got %#v", snapshot)
	}
}

func TestEvaluateMonitoredDeviceCapabilityWarnings(t *testing.T) {
	now := time.Now()
	device := healthyMonitoredDevice(now)
	device.AgentVersion = "1.0.39"
	device.Privileged = false
	device.InstallMode = "user"
	snapshot := evaluateMonitoredDevice(device, now)
	if snapshot.Status != "warning" || !slices.Contains(snapshot.ProblemCodes, "agent_outdated") || !slices.Contains(snapshot.ProblemCodes, "agent_unprivileged") {
		t.Fatalf("missing capability warnings: %#v", snapshot)
	}
}

func TestNormalizeMonitorTargetPolicies(t *testing.T) {
	input := monitorTargetInput{Name: "Router", GatewayDeviceID: "device", Host: "192.168.1.1", Ports: []int{443, 80, 443}, SuccessPolicy: "ANY", IntervalSeconds: 60}
	normalized, err := normalizeMonitorTarget(input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.SuccessPolicy != "any" || !slices.Equal(normalized.Ports, []int{443, 80}) {
		t.Fatalf("unexpected normalization: %#v", normalized)
	}
	input.SuccessPolicy = "majority"
	if _, err := normalizeMonitorTarget(input); err == nil {
		t.Fatal("unsupported success policy must be rejected")
	}
}
