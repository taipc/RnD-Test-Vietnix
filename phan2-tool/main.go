package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

const (
	GiB                  = 1024 * 1024 * 1024
	MinValidNodeMemBytes = 1 * GiB
)

type RawClusterSnapshot struct {
	Cluster     string    `json:"cluster"`
	CollectedAt string    `json:"collected_at"`
	Source      string    `json:"source"`
	Nodes       []RawNode `json:"nodes"`
	VMs         []RawVM   `json:"vms"`
}

type RawNode struct {
	Node     string `json:"node"`
	Status   string `json:"status"`
	CPUCores *int   `json:"cpu_cores"`
	MemTotal *int64 `json:"mem_total"`
	Note     string `json:"note"`
}

type RawVM struct {
	VMID     *int     `json:"vmid"`
	Name     string   `json:"name"`
	Node     string   `json:"node"`
	Status   string   `json:"status"`
	VCPUs    *int     `json:"vcpus"`
	CPUUsage *float64 `json:"cpu_usage"`
	MemMax   *int64   `json:"mem_max"`
	MemUsed  *int64   `json:"mem_used"`
}

type VMRecord struct {
	VMID     int
	Name     string
	Node     string
	Status   string
	VCPUs    int
	CPUUsage float64
	MemMax   int64
	MemUsed  int64
}

func (vm VMRecord) UsedCores() float64 {
	if vm.Status != "running" {
		return 0.0
	}
	return float64(vm.VCPUs) * vm.CPUUsage
}

func (vm VMRecord) ActiveMemUsed() int64 {
	if vm.Status != "running" {
		return 0
	}
	return vm.MemUsed
}

type NodeStats struct {
	Node            string
	Status          string
	CPUCores        int
	MemTotal        int64
	Note            string
	InvalidHardware bool
	VMs             []VMRecord
}

func (ns *NodeStats) Clone() *NodeStats {
	cp := *ns
	cp.VMs = make([]VMRecord, len(ns.VMs))
	copy(cp.VMs, ns.VMs)
	return &cp
}

func (ns *NodeStats) RunningVMsCount() int {
	cnt := 0
	for _, vm := range ns.VMs {
		if vm.Status == "running" {
			cnt++
		}
	}
	return cnt
}

func (ns *NodeStats) StoppedVMsCount() int {
	cnt := 0
	for _, vm := range ns.VMs {
		if vm.Status != "running" {
			cnt++
		}
	}
	return cnt
}

func (ns *NodeStats) CPUUsedCores() float64 {
	sum := 0.0
	for _, vm := range ns.VMs {
		sum += vm.UsedCores()
	}
	return sum
}

func (ns *NodeStats) CPUUsagePct() float64 {
	if ns.CPUCores <= 0 {
		return 0.0
	}
	return (ns.CPUUsedCores() / float64(ns.CPUCores)) * 100.0
}

func (ns *NodeStats) MemUsedBytes() int64 {
	var sum int64
	for _, vm := range ns.VMs {
		sum += vm.ActiveMemUsed()
	}
	return sum
}

func (ns *NodeStats) MemUsagePct() float64 {
	if ns.MemTotal <= 0 || ns.InvalidHardware {
		return 0.0
	}
	return (float64(ns.MemUsedBytes()) / float64(ns.MemTotal)) * 100.0
}

func (ns *NodeStats) VCPUsRunning() int {
	sum := 0
	for _, vm := range ns.VMs {
		if vm.Status == "running" {
			sum += vm.VCPUs
		}
	}
	return sum
}

func (ns *NodeStats) VCPUsTotal() int {
	sum := 0
	for _, vm := range ns.VMs {
		sum += vm.VCPUs
	}
	return sum
}

func (ns *NodeStats) VCPUOvercommitRunning() float64 {
	if ns.CPUCores <= 0 {
		return 0.0
	}
	return float64(ns.VCPUsRunning()) / float64(ns.CPUCores)
}

func (ns *NodeStats) VCPUOvercommitTotal() float64 {
	if ns.CPUCores <= 0 {
		return 0.0
	}
	return float64(ns.VCPUsTotal()) / float64(ns.CPUCores)
}

func (ns *NodeStats) EvaluateOverload(cpuThreshold, memThreshold, overcommitThreshold float64) (bool, []string) {
	if ns.Status != "online" {
		return false, nil
	}
	var reasons []string
	if ns.CPUUsagePct() > cpuThreshold {
		reasons = append(reasons, fmt.Sprintf("CPU>%.0f%% (%.1f%%)", cpuThreshold, ns.CPUUsagePct()))
	}
	if !ns.InvalidHardware && ns.MemUsagePct() > memThreshold {
		reasons = append(reasons, fmt.Sprintf("RAM>%.0f%% (%.1f%%)", memThreshold, ns.MemUsagePct()))
	}
	if ns.VCPUOvercommitRunning() > overcommitThreshold {
		reasons = append(reasons, fmt.Sprintf("vCPU_OVERCOMMIT>%.1fx (%.2fx)", overcommitThreshold, ns.VCPUOvercommitRunning()))
	}
	return len(reasons) > 0, reasons
}

type MigrationStep struct {
	Step              int     `json:"step"`
	VMID              int     `json:"vmid"`
	VMName            string  `json:"vm_name"`
	FromNode          string  `json:"from_node"`
	ToNode            string  `json:"to_node"`
	VCPUs             int     `json:"vcpus"`
	CPUCoresMoved     float64 `json:"cpu_cores_moved"`
	MemUsedBytesMoved int64   `json:"mem_used_bytes_moved"`
	MemUsedGiBMoved   float64 `json:"mem_used_gib_moved"`
	TriggerReason     string  `json:"trigger_reason"`
	CommandHint       string  `json:"command_hint"`
}

type NodeReportJSON struct {
	Node                       string   `json:"node"`
	Status                     string   `json:"status"`
	State                      string   `json:"state"`
	Overloaded                 bool     `json:"overloaded"`
	InvalidHardware            bool     `json:"invalid_hardware"`
	OverloadReasons            []string `json:"overload_reasons"`
	Note                       string   `json:"note,omitempty"`
	VMsRunning                 int      `json:"vms_running"`
	VMsStopped                 int      `json:"vms_stopped"`
	CPUCoresTotal              int      `json:"cpu_cores_total"`
	CPUCoresUsed               float64  `json:"cpu_cores_used"`
	CPUUsagePercent            float64  `json:"cpu_usage_percent"`
	MemTotalBytes              int64    `json:"mem_total_bytes"`
	MemUsedBytes               int64    `json:"mem_used_bytes"`
	MemTotalGiB                float64  `json:"mem_total_gib"`
	MemUsedGiB                 float64  `json:"mem_used_gib"`
	MemUsagePercent            *float64 `json:"mem_usage_percent"`
	VCPUsRunning               int      `json:"vcpus_running"`
	VCPUsTotal                 int      `json:"vcpus_total"`
	VCPUOvercommitRunningRatio float64  `json:"vcpu_overcommit_running_ratio"`
	VCPUOvercommitTotalRatio   float64  `json:"vcpu_overcommit_total_ratio"`
}

type ClusterReportJSON struct {
	Cluster              string           `json:"cluster"`
	CollectedAt          string           `json:"collected_at"`
	Thresholds           map[string]any   `json:"thresholds"`
	AnomaliesAndWarnings []string         `json:"anomalies_and_warnings"`
	NodesBefore          []NodeReportJSON `json:"nodes_before"`
	MigrationPlan        []MigrationStep  `json:"migration_plan"`
	NodesAfter           []NodeReportJSON `json:"nodes_after"`
	PlannerNotes         []string         `json:"planner_notes"`
}

func round2(val float64) float64 {
	return math.Round(val*100) / 100
}

func ParseAndValidate(rawBytes []byte, fixUnitAnomaly bool) (string, string, []*NodeStats, []string, error) {
	var raw RawClusterSnapshot
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		return "", "", nil, nil, fmt.Errorf("lỗi cú pháp JSON: %w", err)
	}
	if len(raw.Nodes) == 0 {
		return "", "", nil, nil, fmt.Errorf("danh sách 'nodes' rỗng hoặc bị thiếu")
	}

	clusterName := raw.Cluster
	if clusterName == "" {
		clusterName = "unknown-cluster"
	}
	collectedAt := raw.CollectedAt
	if collectedAt == "" {
		collectedAt = "unknown-time"
	}

	var warnings []string
	var nodeOrder []*NodeStats
	nodeMap := make(map[string]*NodeStats)

	for idx, n := range raw.Nodes {
		name := strings.TrimSpace(n.Node)
		if name == "" {
			warnings = append(warnings, fmt.Sprintf("[Node #%d] Thiếu tên 'node' hợp lệ, đã bỏ qua.", idx))
			continue
		}
		if _, exists := nodeMap[name]; exists {
			warnings = append(warnings, fmt.Sprintf("[Node %s] Trùng lặp tên node, bỏ qua bản ghi thứ hai.", name))
			continue
		}

		status := strings.ToLower(strings.TrimSpace(n.Status))
		if status == "" {
			status = "unknown"
		}

		cores := 0
		if n.CPUCores == nil || *n.CPUCores <= 0 {
			warnings = append(warnings, fmt.Sprintf("[Node %s] 'cpu_cores' bị thiếu hoặc <= 0, gán bằng 0.", name))
		} else {
			cores = *n.CPUCores
		}

		var memTotal int64
		invalidHW := false
		if n.MemTotal == nil || *n.MemTotal <= 0 {
			warnings = append(warnings, fmt.Sprintf("[Node %s] 'mem_total' bị thiếu hoặc <= 0, gắn cờ INVALID_HARDWARE.", name))
			invalidHW = true
		} else if *n.MemTotal < MinValidNodeMemBytes {
			if fixUnitAnomaly {
				memTotal = (*n.MemTotal) * 1024 * 1024
				warnings = append(warnings, fmt.Sprintf("[Node %s] 'mem_total'=%d bytes (<1 GiB) nghi ngờ sai đơn vị KiB -> đã quy đổi thành %.0f GiB.", name, *n.MemTotal, float64(memTotal)/float64(GiB)))
			} else {
				memTotal = *n.MemTotal
				invalidHW = true
				warnings = append(warnings, fmt.Sprintf("[Node %s] 'mem_total'=%d bytes (%.0f KiB < 1 GiB) bất thường so với tổng RAM các VM trên node; đã gắn cờ INVALID_HARDWARE và khóa chiều nhận VM.", name, memTotal, float64(memTotal)/1024.0))
			}
		} else {
			memTotal = *n.MemTotal
		}

		if status != "online" {
			noteSuffix := ""
			if n.Note != "" {
				noteSuffix = fmt.Sprintf(" (ghi chú: %s)", n.Note)
			}
			warnings = append(warnings, fmt.Sprintf("[Node %s] Node đang ở trạng thái '%s'%s -> loại khỏi danh sách node đích nhận live-migrate.", name, status, noteSuffix))
		}

		st := &NodeStats{
			Node:            name,
			Status:          status,
			CPUCores:        cores,
			MemTotal:        memTotal,
			Note:            n.Note,
			InvalidHardware: invalidHW,
		}
		nodeMap[name] = st
		nodeOrder = append(nodeOrder, st)
	}

	seenVMIDs := make(map[int]string)
	for idx, v := range raw.VMs {
		if v.VMID == nil || *v.VMID <= 0 {
			warnings = append(warnings, fmt.Sprintf("[VM #%d (%s)] 'vmid' bị thiếu hoặc không hợp lệ, đã bỏ qua.", idx, v.Name))
			continue
		}
		vmid := *v.VMID
		vmName := strings.TrimSpace(v.Name)
		if vmName == "" {
			vmName = fmt.Sprintf("vm-%d", vmid)
		}

		if prevNode, dup := seenVMIDs[vmid]; dup {
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] Trùng lặp vmid=%d trên node '%s' (đã tồn tại trên '%s'), bỏ qua bản ghi trùng.", vmid, vmName, vmid, v.Node, prevNode))
			continue
		}

		targetNode, ok := nodeMap[v.Node]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] Tham chiếu tới node '%s' không tồn tại trong danh sách nodes, đã bỏ qua.", vmid, vmName, v.Node))
			continue
		}

		if v.VCPUs == nil || *v.VCPUs <= 0 {
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'vcpus' bị thiếu hoặc <= 0, đã bỏ qua.", vmid, vmName))
			continue
		}
		vcpus := *v.VCPUs

		status := strings.ToLower(strings.TrimSpace(v.Status))
		if status == "" {
			status = "unknown"
		}

		cpuUsage := 0.0
		if v.CPUUsage == nil || *v.CPUUsage < 0 {
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'cpu_usage' bị thiếu hoặc âm, gán mặc định 0.0.", vmid, vmName))
		} else if *v.CPUUsage > 1.0 {
			if *v.CPUUsage <= 100.0 {
				cpuUsage = *v.CPUUsage / 100.0
				warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'cpu_usage'=%.1f vượt thang [0..1] (ghi nhầm theo phần trăm %%) -> đã chuẩn hóa về %.3f.", vmid, vmName, *v.CPUUsage, cpuUsage))
			} else {
				cpuUsage = 1.0
				warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'cpu_usage'=%.1f vượt quá 100%% -> giới hạn (clamp) về 1.0.", vmid, vmName, *v.CPUUsage))
			}
		} else {
			cpuUsage = *v.CPUUsage
		}

		if v.MemMax == nil || *v.MemMax <= 0 {
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'mem_max' bị thiếu hoặc <= 0, đã bỏ qua.", vmid, vmName))
			continue
		}
		memMax := *v.MemMax

		var memUsed int64
		if v.MemUsed == nil {
			if status == "running" {
				memUsed = memMax
			}
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'mem_used' bị null trên node '%s' -> fallback an toàn bằng 'mem_max' (%.1f GiB).", vmid, vmName, v.Node, float64(memUsed)/float64(GiB)))
		} else if *v.MemUsed < 0 {
			if status == "running" {
				memUsed = memMax
			}
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'mem_used' âm -> fallback bằng 'mem_max'.", vmid, vmName))
		} else if *v.MemUsed > memMax {
			memUsed = memMax
			warnings = append(warnings, fmt.Sprintf("[VM %d (%s)] 'mem_used' > 'mem_max' -> giới hạn về 'mem_max'.", vmid, vmName))
		} else {
			memUsed = *v.MemUsed
		}

		seenVMIDs[vmid] = v.Node
		targetNode.VMs = append(targetNode.VMs, VMRecord{
			VMID:     vmid,
			Name:     vmName,
			Node:     v.Node,
			Status:   status,
			VCPUs:    vcpus,
			CPUUsage: cpuUsage,
			MemMax:   memMax,
			MemUsed:  memUsed,
		})
	}

	return clusterName, collectedAt, nodeOrder, warnings, nil
}

func PlanMigrations(
	initialNodes []*NodeStats,
	cpuThreshold, memThreshold, overcommitThreshold, safeMargin float64,
) ([]*NodeStats, []MigrationStep, []string) {
	simNodes := make([]*NodeStats, len(initialNodes))
	for i, n := range initialNodes {
		simNodes[i] = n.Clone()
	}

	var steps []MigrationStep
	var notes []string

	targetCPULimit := math.Max(10.0, cpuThreshold-safeMargin)
	targetMemLimit := math.Max(10.0, memThreshold-safeMargin)

	for _, st := range simNodes {
		if st.InvalidHardware && st.Status == "online" {
			notes = append(notes, fmt.Sprintf("Node '%s' có mem_total bất thường (%d bytes) nên bị khóa chiều nhận VM; chỉ migrate bớt VM nếu vượt ngưỡng CPU hoặc vCPU overcommit.", st.Node, st.MemTotal))
		}
	}

	type candidate struct {
		node         *NodeStats
		reasons      []string
		physOverload bool
		severity     float64
	}

	for iter := 0; iter < 200; iter++ {
		var overloaded []candidate
		for _, st := range simNodes {
			isOv, reasons := st.EvaluateOverload(cpuThreshold, memThreshold, overcommitThreshold)
			if !isOv || len(reasons) == 0 {
				continue
			}
			phys := false
			for _, r := range reasons {
				if strings.HasPrefix(r, "CPU>") || strings.HasPrefix(r, "RAM>") {
					phys = true
					break
				}
			}
			sev := st.CPUUsagePct() - cpuThreshold
			if !st.InvalidHardware {
				if mSev := st.MemUsagePct() - memThreshold; mSev > sev {
					sev = mSev
				}
			}
			if vSev := (st.VCPUOvercommitRunning() - overcommitThreshold) * 20.0; vSev > sev {
				sev = vSev
			}
			overloaded = append(overloaded, candidate{
				node:         st,
				reasons:      reasons,
				physOverload: phys,
				severity:     sev,
			})
		}

		if len(overloaded) == 0 {
			break
		}

		sort.SliceStable(overloaded, func(i, j int) bool {
			if overloaded[i].physOverload != overloaded[j].physOverload {
				return overloaded[i].physOverload
			}
			return overloaded[i].severity > overloaded[j].severity
		})

		movedAny := false
		for _, cand := range overloaded {
			src := cand.node
			needMem, needCPU, needVCPU := false, false, false
			for _, r := range cand.reasons {
				if strings.HasPrefix(r, "RAM>") {
					needMem = true
				}
				if strings.HasPrefix(r, "CPU>") {
					needCPU = true
				}
				if strings.HasPrefix(r, "vCPU_OVERCOMMIT>") {
					needVCPU = true
				}
			}

			var runningIdx []int
			for idx, vm := range src.VMs {
				if vm.Status == "running" {
					runningIdx = append(runningIdx, idx)
				}
			}
			if len(runningIdx) == 0 {
				continue
			}

			vmScore := func(vm VMRecord) float64 {
				score := 0.0
				if needMem && src.MemTotal > 0 && !src.InvalidHardware {
					score += (float64(vm.ActiveMemUsed()) / float64(src.MemTotal)) * 100.0 * 2.5
				}
				if needCPU && src.CPUCores > 0 {
					score += (vm.UsedCores() / float64(src.CPUCores)) * 100.0 * 2.0
				}
				if needVCPU && src.CPUCores > 0 {
					score += (float64(vm.VCPUs) / float64(src.CPUCores)) * 100.0 * 2.0
				}
				return score
			}

			sort.SliceStable(runningIdx, func(i, j int) bool {
				return vmScore(src.VMs[runningIdx[i]]) > vmScore(src.VMs[runningIdx[j]])
			})

			for _, vmPos := range runningIdx {
				vm := src.VMs[vmPos]
				var bestDst *NodeStats
				bestDstScore := math.MaxFloat64

				for _, dst := range simNodes {
					if dst.Node == src.Node {
						continue
					}
					if dst.Status != "online" || dst.InvalidHardware {
						continue
					}
					if dst.CPUCores <= 0 || dst.MemTotal <= 0 {
						continue
					}

					newCPUPct := ((dst.CPUUsedCores() + vm.UsedCores()) / float64(dst.CPUCores)) * 100.0
					newMemPct := (float64(dst.MemUsedBytes()+vm.ActiveMemUsed()) / float64(dst.MemTotal)) * 100.0
					newOvercommit := float64(dst.VCPUsRunning()+vm.VCPUs) / float64(dst.CPUCores)

					if newCPUPct <= targetCPULimit && newMemPct <= targetMemLimit && newOvercommit <= overcommitThreshold {
						dstScore := math.Max(newCPUPct, newMemPct) + (newOvercommit * 10.0)
						if dstScore < bestDstScore {
							bestDstScore = dstScore
							bestDst = dst
						}
					}
				}

				if bestDst != nil {
					src.VMs = append(src.VMs[:vmPos], src.VMs[vmPos+1:]...)
					movedVM := vm
					movedVM.Node = bestDst.Node
					bestDst.VMs = append(bestDst.VMs, movedVM)

					steps = append(steps, MigrationStep{
						Step:              len(steps) + 1,
						VMID:              vm.VMID,
						VMName:            vm.Name,
						FromNode:          src.Node,
						ToNode:            bestDst.Node,
						VCPUs:             vm.VCPUs,
						CPUCoresMoved:     round2(vm.UsedCores()),
						MemUsedBytesMoved: vm.ActiveMemUsed(),
						MemUsedGiBMoved:   round2(float64(vm.ActiveMemUsed()) / float64(GiB)),
						TriggerReason:     strings.Join(cand.reasons, ", "),
						CommandHint:       fmt.Sprintf("qm migrate %d %s --online", vm.VMID, bestDst.Node),
					})
					movedAny = true
					break
				}
			}

			if movedAny {
				break
			}
		}

		if !movedAny {
			for _, cand := range overloaded {
				notes = append(notes, fmt.Sprintf("Không tìm được node đích đủ tài nguyên an toàn để tiếp tục giảm tải cho '%s' (%s).", cand.node.Node, strings.Join(cand.reasons, ", ")))
			}
			break
		}
	}

	return simNodes, steps, notes
}

func serializeNodeStats(st *NodeStats, cpuThreshold, memThreshold, overcommitThreshold float64) NodeReportJSON {
	isOv, reasons := st.EvaluateOverload(cpuThreshold, memThreshold, overcommitThreshold)
	if reasons == nil {
		reasons = []string{}
	}

	stateLabel := "HEALTHY"
	if st.Status != "online" {
		stateLabel = "OFFLINE"
	} else if isOv {
		if st.InvalidHardware {
			stateLabel = "OVERLOADED / INVALID_HW"
		} else {
			stateLabel = "OVERLOADED"
		}
	} else if st.InvalidHardware {
		stateLabel = "INVALID_HW"
	}

	var memPctPtr *float64
	if !st.InvalidHardware {
		v := round2(st.MemUsagePct())
		memPctPtr = &v
	}

	return NodeReportJSON{
		Node:                       st.Node,
		Status:                     st.Status,
		State:                      stateLabel,
		Overloaded:                 isOv,
		InvalidHardware:            st.InvalidHardware,
		OverloadReasons:            reasons,
		Note:                       st.Note,
		VMsRunning:                 st.RunningVMsCount(),
		VMsStopped:                 st.StoppedVMsCount(),
		CPUCoresTotal:              st.CPUCores,
		CPUCoresUsed:               round2(st.CPUUsedCores()),
		CPUUsagePercent:            round2(st.CPUUsagePct()),
		MemTotalBytes:              st.MemTotal,
		MemUsedBytes:               st.MemUsedBytes(),
		MemTotalGiB:                round2(float64(st.MemTotal) / float64(GiB)),
		MemUsedGiB:                 round2(float64(st.MemUsedBytes()) / float64(GiB)),
		MemUsagePercent:            memPctPtr,
		VCPUsRunning:               st.VCPUsRunning(),
		VCPUsTotal:                 st.VCPUsTotal(),
		VCPUOvercommitRunningRatio: round2(st.VCPUOvercommitRunning()),
		VCPUOvercommitTotalRatio:   round2(st.VCPUOvercommitTotal()),
	}
}

func BuildReport(
	clusterName, collectedAt string,
	initialNodes, afterNodes []*NodeStats,
	steps []MigrationStep,
	warnings, notes []string,
	cpuThreshold, memThreshold, overcommitThreshold float64,
) ClusterReportJSON {
	if warnings == nil {
		warnings = []string{}
	}
	if steps == nil {
		steps = []MigrationStep{}
	}
	if notes == nil {
		notes = []string{}
	}

	beforeJSON := make([]NodeReportJSON, len(initialNodes))
	for i, st := range initialNodes {
		beforeJSON[i] = serializeNodeStats(st, cpuThreshold, memThreshold, overcommitThreshold)
	}
	afterJSON := make([]NodeReportJSON, len(afterNodes))
	for i, st := range afterNodes {
		afterJSON[i] = serializeNodeStats(st, cpuThreshold, memThreshold, overcommitThreshold)
	}

	return ClusterReportJSON{
		Cluster:     clusterName,
		CollectedAt: collectedAt,
		Thresholds: map[string]any{
			"cpu_usage_percent":     cpuThreshold,
			"mem_usage_percent":     memThreshold,
			"vcpu_overcommit_ratio": overcommitThreshold,
		},
		AnomaliesAndWarnings: warnings,
		NodesBefore:          beforeJSON,
		MigrationPlan:        steps,
		NodesAfter:           afterJSON,
		PlannerNotes:         notes,
	}
}

func printNodeTable(rows []NodeReportJSON) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Node\tStatus\tVMs(Run/All)\tCPU Thuc Dung\tRAM Thuc Dung\tvCPU Overcommit (Run/All)\tDanh Gia")
	fmt.Fprintln(w, "----\t------\t------------\t-------------\t-------------\t-------------------------\t--------")
	for _, r := range rows {
		vmsStr := fmt.Sprintf("%d/%d", r.VMsRunning, r.VMsRunning+r.VMsStopped)
		cpuStr := fmt.Sprintf("%.2f/%dc (%.1f%%)", r.CPUCoresUsed, r.CPUCoresTotal, r.CPUUsagePercent)
		var memStr string
		if r.InvalidHardware || r.MemUsagePercent == nil {
			memStr = fmt.Sprintf("%.1fG / %dB (ERR)", r.MemUsedGiB, r.MemTotalBytes)
		} else {
			memStr = fmt.Sprintf("%.1f/%.0fG (%.1f%%)", r.MemUsedGiB, r.MemTotalGiB, *r.MemUsagePercent)
		}
		overStr := fmt.Sprintf("%d/%d (%.2fx | %.2fx)", r.VCPUsRunning, r.CPUCoresTotal, r.VCPUOvercommitRunningRatio, r.VCPUOvercommitTotalRatio)

		var stateStr string
		if len(r.OverloadReasons) > 0 {
			stateStr = fmt.Sprintf("[!] %s: %s", r.State, strings.Join(r.OverloadReasons, ", "))
		} else if r.State == "INVALID_HW" {
			stateStr = "[?] INVALID_HW (mem_total loi)"
		} else if r.State == "OFFLINE" {
			if r.Note != "" {
				stateStr = fmt.Sprintf("[-] OFFLINE (%s)", r.Note)
			} else {
				stateStr = "[-] OFFLINE"
			}
		} else {
			stateStr = "[OK] HEALTHY"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Node, r.Status, vmsStr, cpuStr, memStr, overStr, stateStr)
	}
	w.Flush()
}

func PrintTableReport(report ClusterReportJSON, cpuThreshold, memThreshold, overcommitThreshold float64) {
	fmt.Println(strings.Repeat("=", 108))
	fmt.Printf(" BAO CAO PHAN TICH TAI CLUSTER PROXMOX VE: %s (Snapshot: %s)\n", report.Cluster, report.CollectedAt)
	fmt.Printf(" Nguong qua tai: CPU > %.0f%% | RAM > %.0f%% | vCPU Overcommit (Running) > %.1fx\n", cpuThreshold, memThreshold, overcommitThreshold)
	fmt.Println(strings.Repeat("=", 108))

	if len(report.AnomaliesAndWarnings) > 0 {
		fmt.Println("\n[1] CANH BAO DU LIEU BAT THUONG (DATA ANOMALIES & SANITIZATION):")
		for idx, warn := range report.AnomaliesAndWarnings {
			fmt.Printf("  %d. %s\n", idx+1, warn)
		}
	}

	fmt.Println("\n[2] MUC TAI CAC NODE TRUOC KHI MIGRATE (CURRENT STATE):")
	printNodeTable(report.NodesBefore)

	fmt.Println("\n[3] KE HOACH DE XUAT LIVE-MIGRATE VM (MIGRATION PLAN):")
	if len(report.MigrationPlan) == 0 {
		fmt.Println("  Khong co de xuat di chuyen VM nao (he thong da can bang).")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
		fmt.Fprintln(w, "Buoc\tVMID\tTen VM\tTu Node\tSang Node\tvCPU\tCPU Thuc\tRAM Thuc\tLy Do Chuyen\tLenh Goi Y (Proxmox CLI)")
		fmt.Fprintln(w, "----\t----\t------\t-------\t---------\t----\t--------\t--------\t------------\t------------------------")
		for _, p := range report.MigrationPlan {
			fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%d\t%.2fc\t%.1f GiB\t%s\t%s\n",
				p.Step, p.VMID, p.VMName, p.FromNode, p.ToNode, p.VCPUs, p.CPUCoresMoved, p.MemUsedGiBMoved, p.TriggerReason, p.CommandHint)
		}
		w.Flush()
	}

	fmt.Println("\n[4] DU KIEN MUC TAI CAC NODE SAU KHI MIGRATE (PROJECTED STATE):")
	printNodeTable(report.NodesAfter)

	if len(report.PlannerNotes) > 0 {
		fmt.Println("\n[5] GHI CHU VAN HANH:")
		for _, note := range report.PlannerNotes {
			fmt.Printf("  * %s\n", note)
		}
	}
}

func main() {
	filePath := flag.String("f", "cluster-pve-hcm-c3.json", "Đường dẫn tới file snapshot JSON của Proxmox cluster")
	outputFormat := flag.String("o", "table", "Định dạng hiển thị kết quả: 'table' hoặc 'json'")
	outFilePath := flag.String("out", "", "Đường dẫn file để lưu kết quả JSON (tùy chọn)")
	cpuThreshold := flag.Float64("cpu-threshold", 80.0, "Ngưỡng quá tải CPU thực dùng (%)")
	memThreshold := flag.Float64("mem-threshold", 80.0, "Ngưỡng quá tải RAM thực dùng (%)")
	overcommitThreshold := flag.Float64("overcommit-threshold", 2.5, "Ngưỡng quá tải tỷ lệ vCPU overcommit (Running VMs)")
	fixUnitAnomaly := flag.Bool("fix-unit-anomaly", false, "Quy đổi mem_total < 1 GiB từ KiB sang GiB")
	flag.Parse()

	rawBytes, err := os.ReadFile(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Không thể đọc file '%s': %v\n", *filePath, err)
		os.Exit(1)
	}

	clusterName, collectedAt, initialNodes, warnings, err := ParseAndValidate(rawBytes, *fixUnitAnomaly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Dữ liệu đầu vào không hợp lệ: %v\n", err)
		os.Exit(1)
	}

	afterNodes, steps, notes := PlanMigrations(
		initialNodes,
		*cpuThreshold,
		*memThreshold,
		*overcommitThreshold,
		2.0,
	)

	report := BuildReport(
		clusterName,
		collectedAt,
		initialNodes,
		afterNodes,
		steps,
		warnings,
		notes,
		*cpuThreshold,
		*memThreshold,
		*overcommitThreshold,
	)

	if strings.ToLower(*outputFormat) == "json" {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
		}

		var enc *json.Encoder
		if *outFilePath != "" {
			file, err := os.Create(*outFilePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Không thể tạo file output: %v\n", err)
				os.Exit(1)
			}
			defer file.Close()
			enc = json.NewEncoder(file)
			fmt.Fprintf(os.Stderr, "Đã lưu kết quả JSON chuẩn UTF-8 vào file: %s\n", *outFilePath)
		} else {
			enc = json.NewEncoder(os.Stdout)
		}

		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)

		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Không thể xuất JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		PrintTableReport(report, *cpuThreshold, *memThreshold, *overcommitThreshold)
	}
}
