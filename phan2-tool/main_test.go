package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// 1. Kiểm thử lỗi cú pháp JSON và cấu trúc rỗng
func TestParseAndValidate_ErrorsAndDefaults(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errContains string
	}{
		{
			name:        "Malformed JSON",
			input:       `{"cluster": "pve", "nodes": [`,
			expectError: true,
			errContains: "lỗi cú pháp JSON",
		},
		{
			name:        "Empty Nodes List",
			input:       `{"cluster": "pve", "nodes": []}`,
			expectError: true,
			errContains: "danh sách 'nodes' rỗng",
		},
		{
			name:        "Missing Cluster and CollectedAt Uses Defaults",
			input:       `{"nodes": [{"node": "n1", "status": "online", "cpu_cores": 8, "mem_total": 17179869184}]}`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster, collected, nodes, _, err := ParseAndValidate([]byte(tc.input), false)
			if tc.expectError {
				if err == nil {
					t.Fatalf("Kỳ vọng lỗi nhưng hàm trả về nil")
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("Kỳ vọng lỗi chứa %q, nhưng nhận được: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Không kỳ vọng lỗi nhưng nhận được: %v", err)
				}
				if cluster != "unknown-cluster" || collected != "unknown-time" {
					t.Errorf("Fallback mặc định sai: cluster=%q, collected=%q", cluster, collected)
				}
				if len(nodes) != 1 {
					t.Errorf("Kỳ vọng 1 node, nhận được %d", len(nodes))
				}
			}
		})
	}
}

// 2. Kiểm thử toàn bộ các bẫy dữ liệu ở cấp độ Node
func TestParseAndValidate_NodeAnomalies(t *testing.T) {
	rawJSON := `{
		"cluster": "test-cluster",
		"collected_at": "2026-10-31T00:00:00Z",
		"nodes": [
			{"node": "   ", "status": "online", "cpu_cores": 16, "mem_total": 34359738368},
			{"node": "n01", "status": "ONLINE", "cpu_cores": 16, "mem_total": 34359738368},
			{"node": "n01", "status": "online", "cpu_cores": 32, "mem_total": 68719476736},
			{"node": "n02", "status": "", "cpu_cores": null, "mem_total": null},
			{"node": "n03", "status": "online", "cpu_cores": -4, "mem_total": 0},
			{"node": "n04", "status": "offline", "cpu_cores": 16, "mem_total": 34359738368, "note": "Bảo trì PSU"},
			{"node": "n06", "status": "online", "cpu_cores": 32, "mem_total": 262144}
		],
		"vms": []
	}`

	t.Run("Default Mode (fixUnitAnomaly = false)", func(t *testing.T) {
		_, _, nodes, warnings, err := ParseAndValidate([]byte(rawJSON), false)
		if err != nil {
			t.Fatalf("Lỗi không mong đợi: %v", err)
		}

		// Có 7 bản ghi nhưng 1 rỗng tên và 1 trùng tên n01 -> còn 5 node
		if len(nodes) != 5 {
			t.Fatalf("Kỳ vọng 5 nodes hợp lệ sau lọc, nhận được %d", len(nodes))
		}

		nodeMap := make(map[string]*NodeStats)
		for _, n := range nodes {
			nodeMap[n.Node] = n
		}

		if nodeMap["n01"].CPUCores != 16 || nodeMap["n01"].Status != "online" {
			t.Errorf("Node n01 phải giữ bản ghi đầu tiên (16 cores, online)")
		}
		if nodeMap["n02"].Status != "unknown" || nodeMap["n02"].CPUCores != 0 || !nodeMap["n02"].InvalidHardware {
			t.Errorf("Node n02 xử lý thiếu dữ liệu sai: %+v", nodeMap["n02"])
		}
		if nodeMap["n03"].CPUCores != 0 || !nodeMap["n03"].InvalidHardware {
			t.Errorf("Node n03 xử lý số âm/0 sai: %+v", nodeMap["n03"])
		}
		if nodeMap["n04"].Status != "offline" {
			t.Errorf("Node n04 phải ở trạng thái offline")
		}
		if !nodeMap["n06"].InvalidHardware || nodeMap["n06"].MemTotal != 262144 {
			t.Errorf("Node n06 (256KiB) phải bị gắn cờ InvalidHardware khi fixUnitAnomaly=false")
		}
		if len(warnings) < 6 {
			t.Errorf("Kỳ vọng ít nhất 6 cảnh báo, nhận được %d", len(warnings))
		}
	})

	t.Run("Auto-Fix KiB Mode (fixUnitAnomaly = true)", func(t *testing.T) {
		_, _, nodes, _, err := ParseAndValidate([]byte(rawJSON), true)
		if err != nil {
			t.Fatalf("Lỗi không mong đợi: %v", err)
		}
		for _, n := range nodes {
			if n.Node == "n06" {
				expectedBytes := int64(262144) * 1024 * 1024 // 256 GiB
				if n.InvalidHardware || n.MemTotal != expectedBytes {
					t.Errorf("Khi bật fixUnitAnomaly, n06 phải có MemTotal=%d và InvalidHardware=false, thực tế: MemTotal=%d, Invalid=%v",
						expectedBytes, n.MemTotal, n.InvalidHardware)
				}
			}
		}
	})
}

// 3. Kiểm thử toàn bộ các bẫy dữ liệu ở cấp độ VM
func TestParseAndValidate_VMAnomalies(t *testing.T) {
	rawJSON := `{
		"cluster": "test-vm-traps",
		"nodes": [
			{"node": "n01", "status": "online", "cpu_cores": 32, "mem_total": 68719476736}
		],
		"vms": [
			{"vmid": null, "name": "no-id", "node": "n01", "status": "running", "vcpus": 2, "cpu_usage": 0.5, "mem_max": 1073741824, "mem_used": 536870912},
			{"vmid": -5, "name": "neg-id", "node": "n01", "status": "running", "vcpus": 2, "cpu_usage": 0.5, "mem_max": 1073741824, "mem_used": 536870912},
			{"vmid": 100, "name": "   ", "node": "n01", "status": "RUNNING", "vcpus": 4, "cpu_usage": 98.0, "mem_max": 4294967296, "mem_used": null},
			{"vmid": 100, "name": "dup-vm", "node": "n01", "status": "running", "vcpus": 4, "cpu_usage": 0.2, "mem_max": 4294967296, "mem_used": 1073741824},
			{"vmid": 101, "name": "orphan-vm", "node": "n99-ghost", "status": "running", "vcpus": 4, "cpu_usage": 0.2, "mem_max": 4294967296, "mem_used": 1073741824},
			{"vmid": 102, "name": "zero-vcpu", "node": "n01", "status": "running", "vcpus": 0, "cpu_usage": 0.2, "mem_max": 4294967296, "mem_used": 1073741824},
			{"vmid": 103, "name": "zero-memmax", "node": "n01", "status": "running", "vcpus": 2, "cpu_usage": 0.2, "mem_max": 0, "mem_used": 1073741824},
			{"vmid": 104, "name": "cpu-over-100-mem-exceed", "node": "n01", "status": "running", "vcpus": 2, "cpu_usage": 150.0, "mem_max": 2147483648, "mem_used": 9999999999},
			{"vmid": 105, "name": "neg-cpu-neg-mem", "node": "n01", "status": "running", "vcpus": 2, "cpu_usage": -0.5, "mem_max": 2147483648, "mem_used": -100},
			{"vmid": 106, "name": "stopped-null-mem", "node": "n01", "status": "stopped", "vcpus": 8, "cpu_usage": 0.8, "mem_max": 8589934592, "mem_used": null}
		]
	}`

	_, _, nodes, _, err := ParseAndValidate([]byte(rawJSON), false)
	if err != nil {
		t.Fatalf("Lỗi parse: %v", err)
	}

	vms := nodes[0].VMs
	// Chỉ các VM 100 (bản đầu), 104, 105, 106 là hợp lệ -> tổng 4 VMs
	if len(vms) != 4 {
		t.Fatalf("Kỳ vọng 4 VMs hợp lệ sau khi lọc bẫy, nhận được %d", len(vms))
	}

	vmMap := make(map[int]VMRecord)
	for _, vm := range vms {
		vmMap[vm.VMID] = vm
	}

	// Kiểm tra VM 100: tên rỗng -> "vm-100", cpu 98.0 -> 0.98, mem_used null (running) -> mem_max
	if vmMap[100].Name != "vm-100" || !almostEqual(vmMap[100].CPUUsage, 0.98) || vmMap[100].MemUsed != 4294967296 {
		t.Errorf("VM 100 chuẩn hóa sai: %+v", vmMap[100])
	}

	// Kiểm tra VM 104: cpu 150.0 -> clamp về 1.0, mem_used > mem_max -> clamp về mem_max
	if !almostEqual(vmMap[104].CPUUsage, 1.0) || vmMap[104].MemUsed != 2147483648 {
		t.Errorf("VM 104 clamp biên trên sai: %+v", vmMap[104])
	}

	// Kiểm tra VM 105: cpu âm -> 0.0, mem_used âm (running) -> fallback về mem_max
	if !almostEqual(vmMap[105].CPUUsage, 0.0) || vmMap[105].MemUsed != 2147483648 {
		t.Errorf("VM 105 xử lý số âm sai: %+v", vmMap[105])
	}

	// Kiểm tra VM 106: trạng thái stopped + mem_used null -> MemUsed = 0, UsedCores() = 0, ActiveMemUsed() = 0
	if vmMap[106].MemUsed != 0 || vmMap[106].UsedCores() != 0 || vmMap[106].ActiveMemUsed() != 0 {
		t.Errorf("VM 106 (stopped) không được chiếm tài nguyên thực dùng: %+v", vmMap[106])
	}
}

// 4. Kiểm thử công thức tính tải và ranh giới ngưỡng quá tải (80.0% vs 80.1%)
func TestNodeStats_CalculationsAndBoundaryThresholds(t *testing.T) {
	node := &NodeStats{
		Node:     "n-boundary",
		Status:   "online",
		CPUCores: 10,
		MemTotal: 100 * GiB,
		VMs: []VMRecord{
			// Running VM dùng đúng 8.0 cores (80%) và 80 GiB RAM (80%), vCPU = 25 (2.5x)
			{VMID: 1, Status: "running", VCPUs: 25, CPUUsage: 0.32, MemMax: 80 * GiB, MemUsed: 80 * GiB},
			// Stopped VM có thông số rất lớn nhưng phải bị bỏ qua khi tính tải running
			{VMID: 2, Status: "stopped", VCPUs: 50, CPUUsage: 1.0, MemMax: 100 * GiB, MemUsed: 100 * GiB},
		},
	}

	if node.RunningVMsCount() != 1 || node.StoppedVMsCount() != 1 {
		t.Errorf("Đếm số lượng VM running/stopped sai")
	}
	if !almostEqual(node.CPUUsagePct(), 80.0) || !almostEqual(node.MemUsagePct(), 80.0) || !almostEqual(node.VCPUOvercommitRunning(), 2.5) {
		t.Errorf("Tính phần trăm tại biên sai: CPU=%.2f%%, RAM=%.2f%%, Overcommit=%.2fx",
			node.CPUUsagePct(), node.MemUsagePct(), node.VCPUOvercommitRunning())
	}

	// Đúng bằng ngưỡng (80%, 80%, 2.5x) -> KHÔNG quá tải (vì điều kiện là > ngưỡng)
	isOv, reasons := node.EvaluateOverload(80.0, 80.0, 2.5)
	if isOv || len(reasons) != 0 {
		t.Errorf("Tại đúng biên 80.0%%/2.5x không được báo quá tải, nhưng nhận được: %v", reasons)
	}

	// Nhích nhẹ ngưỡng xuống 79.9% / 2.4x -> Phải báo quá tải cả 3 tiêu chí
	isOv2, reasons2 := node.EvaluateOverload(79.9, 79.9, 2.4)
	if !isOv2 || len(reasons2) != 3 {
		t.Errorf("Kỳ vọng báo quá tải đủ 3 lý do (CPU, RAM, vCPU_OVERCOMMIT), nhận được: %v", reasons2)
	}

	// Nếu Node chuyển sang offline -> Không đánh giá quá tải
	node.Status = "offline"
	isOvOffline, _ := node.EvaluateOverload(10.0, 10.0, 1.0)
	if isOvOffline {
		t.Errorf("Node offline không được báo trạng thái quá tải")
	}
}

// 5. Kiểm thử hàm Clone() đảm bảo tính bất biến (Immutability) của dữ liệu gốc
func TestNodeStats_CloneDeepCopy(t *testing.T) {
	orig := &NodeStats{
		Node:     "n1",
		Status:   "online",
		CPUCores: 8,
		MemTotal: 16 * GiB,
		VMs:      []VMRecord{{VMID: 10, Name: "vm-a", Status: "running"}},
	}
	cloned := orig.Clone()
	cloned.VMs[0].Name = "modified-name"
	cloned.VMs = append(cloned.VMs, VMRecord{VMID: 11, Name: "vm-b"})

	if orig.VMs[0].Name != "vm-a" || len(orig.VMs) != 1 {
		t.Errorf("Hàm Clone() không thực hiện deep copy trên slice VMs!")
	}
}

// 6. Kiểm thử thuật toán điều phối Live-Migrate (Greedy Best-Fit Decreasing & Fail-safe)
func TestPlanMigrations_Scenarios(t *testing.T) {
	t.Run("Greedy Best-Fit Picks Heaviest VM First and Skips Invalid/Offline Nodes", func(t *testing.T) {
		nodes := []*NodeStats{
			{
				Node:     "src-overloaded",
				Status:   "online",
				CPUCores: 10,
				MemTotal: 100 * GiB,
				VMs: []VMRecord{
					{VMID: 101, Name: "small-vm", Node: "src-overloaded", Status: "running", VCPUs: 2, CPUUsage: 0.1, MemMax: 15 * GiB, MemUsed: 15 * GiB},
					{VMID: 102, Name: "heavy-vm", Node: "src-overloaded", Status: "running", VCPUs: 4, CPUUsage: 0.2, MemMax: 30 * GiB, MemUsed: 30 * GiB},
					{VMID: 103, Name: "base-vm", Node: "src-overloaded", Status: "running", VCPUs: 4, CPUUsage: 0.2, MemMax: 45 * GiB, MemUsed: 45 * GiB},
				},
			},
			{
				Node:     "dst-offline",
				Status:   "offline",
				CPUCores: 64,
				MemTotal: 500 * GiB,
				Note:     "maintenance",
			},
			{
				Node:            "dst-invalid-hw",
				Status:          "online",
				CPUCores:        64,
				MemTotal:        262144,
				InvalidHardware: true,
			},
			{
				Node:     "dst-healthy",
				Status:   "online",
				CPUCores: 32,
				MemTotal: 100 * GiB,
				VMs: []VMRecord{
					{VMID: 201, Name: "dst-vm", Node: "dst-healthy", Status: "running", VCPUs: 4, CPUUsage: 0.1, MemMax: 20 * GiB, MemUsed: 20 * GiB},
				},
			},
		}

		afterNodes, steps, notes := PlanMigrations(nodes, 80.0, 80.0, 2.5, 2.0)

		if len(steps) != 1 {
			t.Fatalf("Thuật toán Greedy phải giải quyết xong chỉ với 1 bước migrate, thực tế: %d bước", len(steps))
		}
		if steps[0].VMID != 103 && steps[0].VMID != 102 {
			t.Errorf("Phải ưu tiên chọn VM ngốn RAM lớn nhất (103 hoặc 102), nhưng lại chọn VM %d", steps[0].VMID)
		}
		if steps[0].ToNode != "dst-healthy" {
			t.Errorf("Node đích phải là 'dst-healthy', thực tế: %s", steps[0].ToNode)
		}

		isOvAfter, _ := afterNodes[0].EvaluateOverload(80.0, 80.0, 2.5)
		if isOvAfter {
			t.Errorf("Node nguồn sau khi migrate vẫn bị quá tải!")
		}

		// Kiểm thử luôn BuildReport và PrintTableReport để phủ kín code hiển thị
		rep := BuildReport("test-c", "now", nodes, afterNodes, steps, []string{"warn-1"}, notes, 80.0, 80.0, 2.5)
		PrintTableReport(rep, 80.0, 80.0, 2.5)
	})

	t.Run("Unresolvable Overload Generates Planner Note", func(t *testing.T) {
		nodes := []*NodeStats{
			{
				Node:     "src-full",
				Status:   "online",
				CPUCores: 8,
				MemTotal: 32 * GiB,
				VMs: []VMRecord{
					{VMID: 1, Name: "monster-vm", Node: "src-full", Status: "running", VCPUs: 24, CPUUsage: 0.95, MemMax: 30 * GiB, MemUsed: 30 * GiB},
				},
			},
			{
				Node:     "dst-tiny",
				Status:   "online",
				CPUCores: 4,
				MemTotal: 16 * GiB,
			},
		}

		afterNodes, steps, notes := PlanMigrations(nodes, 80.0, 80.0, 2.5, 2.0)
		if len(steps) != 0 {
			t.Errorf("Không thể migrate khi node đích quá nhỏ, nhưng vẫn có %d bước", len(steps))
		}
		if len(notes) == 0 || !strings.Contains(notes[0], "Không tìm được node đích") {
			t.Errorf("Phải ghi nhận cảnh báo vào PlannerNotes khi không thể cứu node quá tải, nhận được: %v", notes)
		}

		rep := BuildReport("test-c2", "now", nodes, afterNodes, nil, nil, nil, 80.0, 80.0, 2.5)
		PrintTableReport(rep, 80.0, 80.0, 2.5)
	})
}

// 7. Kiểm thử hàm đóng gói JSON (BuildReport) không bị mảng null và không escape ký tự HTML
func TestBuildReport_JSONSerialization(t *testing.T) {
	nodes := []*NodeStats{
		{
			Node:            "n-hw-err",
			Status:          "online",
			CPUCores:        16,
			MemTotal:        262144,
			InvalidHardware: true,
			VMs: []VMRecord{
				{VMID: 99, Name: "cpu-hog", Status: "running", VCPUs: 16, CPUUsage: 0.95, MemMax: 4 * GiB, MemUsed: 4 * GiB},
			},
		},
	}

	report := BuildReport("c1", "now", nodes, nodes, nil, nil, nil, 80.0, 80.0, 2.5)

	if report.AnomaliesAndWarnings == nil || report.MigrationPlan == nil || report.PlannerNotes == nil {
		t.Errorf("Các mảng cảnh báo/migrate/ghi chú không được phép nil")
	}
	if report.NodesBefore[0].MemUsagePercent != nil {
		t.Errorf("Node có InvalidHardware phải có MemUsagePercent = nil (null trong JSON)")
	}
	if report.NodesBefore[0].State != "OVERLOADED / INVALID_HW" {
		t.Errorf("Trạng thái State kỳ vọng 'OVERLOADED / INVALID_HW', nhận được %q", report.NodesBefore[0].State)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"reason": "CPU>80%"})
	if strings.Contains(buf.String(), `\u003e`) {
		t.Errorf("JSON bị escape ký tự '>' thành \\u003e")
	}
}

// 8. Kiểm thử tích hợp (Integration Test) trực tiếp trên file dữ liệu thực tế của đề bài
func TestIntegration_RealClusterSnapshot(t *testing.T) {
	data, err := os.ReadFile("cluster-pve-hcm-c3.json")
	if err != nil {
		t.Skip("Bỏ qua test tích hợp vì không tìm thấy file cluster-pve-hcm-c3.json")
	}

	cluster, _, initialNodes, warnings, err := ParseAndValidate(data, false)
	if err != nil {
		t.Fatalf("Parse file thực tế thất bại: %v", err)
	}
	if cluster != "pve-hcm-c3" {
		t.Errorf("Tên cluster sai: %s", cluster)
	}
	if len(warnings) < 5 {
		t.Errorf("Kỳ vọng phát hiện ít nhất 5 cảnh báo dị thường từ file thật, nhận được %d", len(warnings))
	}

	afterNodes, steps, _ := PlanMigrations(initialNodes, 80.0, 80.0, 2.5, 2.0)
	if len(steps) == 0 {
		t.Fatalf("Kỳ vọng có kế hoạch migrate cho cụm thực tế")
	}

	for _, n := range afterNodes {
		if n.Status == "online" && !n.InvalidHardware {
			isOv, reasons := n.EvaluateOverload(80.0, 80.0, 2.5)
			if isOv {
				t.Errorf("Node %s vẫn còn quá tải sau khi chạy kế hoạch migrate: %v", n.Node, reasons)
			}
		}
	}
}
