package main

import (
	"math"
	"testing"
)

const sampleTestJSON = `{
  "cluster": "test-cluster",
  "collected_at": "2026-10-27T09:00:00+07:00",
  "nodes": [
    {"node": "node-01", "status": "online", "cpu_cores": 32, "mem_total": 137438953472},
    {"node": "node-02", "status": "online", "cpu_cores": 32, "mem_total": 137438953472},
    {"node": "node-03", "status": "offline", "cpu_cores": 32, "mem_total": 137438953472, "note": "maintenance"},
    {"node": "node-corrupt", "status": "online", "cpu_cores": 32, "mem_total": 262144}
  ],
  "vms": [
    {"vmid": 101, "name": "heavy-vm-1", "node": "node-01", "status": "running", "vcpus": 20, "cpu_usage": 0.9, "mem_max": 85899345920, "mem_used": 75161927680},
    {"vmid": 102, "name": "heavy-vm-2", "node": "node-01", "status": "running", "vcpus": 10, "cpu_usage": 0.9, "mem_max": 48318382080, "mem_used": 42949672960},
    {"vmid": 103, "name": "pct-cpu-vm", "node": "node-02", "status": "running", "vcpus": 4, "cpu_usage": 50.0, "mem_max": 17179869184, "mem_used": 8589934592},
    {"vmid": 101, "name": "heavy-vm-1", "node": "node-02", "status": "running", "vcpus": 20, "cpu_usage": 0.9, "mem_max": 85899345920, "mem_used": 75161927680},
    {"vmid": 104, "name": "null-mem-vm", "node": "node-02", "status": "running", "vcpus": 4, "cpu_usage": 0.25, "mem_max": 10737418240, "mem_used": null},
    {"vmid": 999, "name": "orphan-vm", "node": "node-99", "status": "running", "vcpus": 4, "cpu_usage": 0.5, "mem_max": 8589934592, "mem_used": 4294967296},
    {"vmid": 105, "name": "stopped-vm", "node": "node-01", "status": "stopped", "vcpus": 16, "cpu_usage": 0, "mem_max": 68719476736, "mem_used": 0}
  ]
}`

func TestParseAndSanitizeAnomalies(t *testing.T) {
	cluster, _, nodes, warnings, err := ParseAndValidate([]byte(sampleTestJSON), false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if cluster != "test-cluster" {
		t.Errorf("Expected cluster 'test-cluster', got '%s'", cluster)
	}
	if len(nodes) != 4 {
		t.Fatalf("Expected 4 nodes, got %d", len(nodes))
	}

	// Node-corrupt phải bị gắn cờ InvalidHardware
	if !nodes[3].InvalidHardware {
		t.Errorf("Expected node-corrupt to be marked InvalidHardware")
	}

	// Bản ghi trùng vmid=101 trên node-02 phải bị bỏ qua
	node2 := nodes[1]
	if len(node2.VMs) != 2 {
		t.Fatalf("Expected 2 valid VMs on node-02, got %d", len(node2.VMs))
	}

	// Kiểm tra chuẩn hóa cpu_usage = 50.0 -> 0.5 trên VM 103
	vm103 := node2.VMs[0]
	if math.Abs(vm103.CPUUsage-0.5) > 1e-6 {
		t.Errorf("Expected vm103 CPUUsage=0.5, got %f", vm103.CPUUsage)
	}

	// Kiểm tra fallback mem_used = null -> mem_max trên VM 104
	vm104 := node2.VMs[1]
	if vm104.MemUsed != 10*GiB {
		t.Errorf("Expected vm104 MemUsed=%d, got %d", 10*GiB, vm104.MemUsed)
	}

	if len(warnings) < 5 {
		t.Errorf("Expected at least 5 anomaly warnings, got %d", len(warnings))
	}
}

func TestNodeMetricsAndStoppedVM(t *testing.T) {
	_, _, nodes, _, err := ParseAndValidate([]byte(sampleTestJSON), false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	n1 := nodes[0]
	if n1.RunningVMsCount() != 2 || n1.StoppedVMsCount() != 1 {
		t.Errorf("Unexpected running/stopped VM count on node-01: %d / %d", n1.RunningVMsCount(), n1.StoppedVMsCount())
	}
	// Running VMs: 101 (18c, 70 GiB) + 102 (9c, 40 GiB) = 27 cores, 110 GiB
	if math.Abs(n1.CPUUsedCores()-27.0) > 1e-6 {
		t.Errorf("Expected 27.0 used cores on node-01, got %f", n1.CPUUsedCores())
	}
	if n1.MemUsedBytes() != 110*GiB {
		t.Errorf("Expected 110 GiB used RAM on node-01, got %d", n1.MemUsedBytes())
	}
	if n1.VCPUsRunning() != 30 || n1.VCPUsTotal() != 46 {
		t.Errorf("Expected vCPUs running=30, total=46; got %d, %d", n1.VCPUsRunning(), n1.VCPUsTotal())
	}
}

func TestMigrationPlannerBalancesClusterSafely(t *testing.T) {
	_, _, nodes, _, err := ParseAndValidate([]byte(sampleTestJSON), false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	afterNodes, steps, _ := PlanMigrations(nodes, 80.0, 80.0, 2.5, 2.0)
	if len(steps) != 1 {
		t.Fatalf("Expected exactly 1 migration step, got %d", len(steps))
	}
	if steps[0].FromNode != "node-01" || steps[0].ToNode != "node-02" {
		t.Errorf("Expected migration from node-01 to node-02, got %s -> %s", steps[0].FromNode, steps[0].ToNode)
	}

	ov1, _ := afterNodes[0].EvaluateOverload(80.0, 80.0, 2.5)
	ov2, _ := afterNodes[1].EvaluateOverload(80.0, 80.0, 2.5)
	if ov1 || ov2 {
		t.Errorf("Expected both node-01 and node-02 to be healthy after migration (ov1=%v, ov2=%v)", ov1, ov2)
	}
}

func TestInvalidJSONHandling(t *testing.T) {
	_, _, _, _, err := ParseAndValidate([]byte(`{invalid json`), false)
	if err == nil {
		t.Errorf("Expected error on malformed JSON, got nil")
	}
}
