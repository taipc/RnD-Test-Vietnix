# Proxmox VE Cluster Load Analyzer & Live-Migration Planner (Go Edition)

Công cụ CLI viết bằng **Go (Golang)** dùng để giám sát mức tải của cụm Proxmox VE từ file snapshot JSON, tự động phát hiện và làm sạch dữ liệu bất thường (data sanitization), đánh dấu các node quá tải và đề xuất kế hoạch `live-migrate` VM nhằm cân bằng tài nguyên toàn cụm.

## 1. Yêu cầu hệ thống & Cấu trúc thư mục

- **Ngôn ngữ:** Go >= 1.20 (chỉ sử dụng Go Standard Library `encoding/json`, `flag`, `text/tabwriter`, `testing`, không phụ thuộc thư viện ngoài).
- **Cấu trúc thư mục `phan2-tool/`:**
  ```text
  phan2-tool/
    go.mod          # Khai báo Go module
    main.go         # Mã nguồn chính (Parser, Sanitizer, Migration Planner, CLI Formatter)
    main_test.go    # Bộ Unit Test tự động kiểm thử logic cốt lõi
    README.md       # Tài liệu hướng dẫn sử dụng và giả định kỹ thuật
  ```

## 2. Hướng dẫn Biên dịch & Chạy Tool

### Chạy trực tiếp hoặc biên dịch binary
```bash
cd phan2-tool
go build -o pve-analyzer main.go
```

### 1) Hiển thị dạng Bảng trực quan trên Terminal (Mặc định)
```bash
./pve-analyzer -f ../phan2-proxmox/cluster-pve-hcm-c3.json -o table
# Hoặc chạy trực tiếp:
go run main.go -f ../phan2-proxmox/cluster-pve-hcm-c3.json -o table
```

### 2) Xuất định dạng JSON chuẩn (để pipe sang `jq` hoặc script tự động hóa)
Khi chạy với `-o json`, toàn bộ cảnh báo dữ liệu bất thường được đẩy ra `stderr`, còn `stdout` chỉ xuất JSON thuần khiết:
```bash
./pve-analyzer -f ../phan2-proxmox/cluster-pve-hcm-c3.json -o json | jq '.migration_plan'
```

### 3) Tùy chỉnh ngưỡng đánh giá quá tải
```bash
./pve-analyzer   -f ../phan2-proxmox/cluster-pve-hcm-c3.json   -cpu-threshold 80   -mem-threshold 80   -overcommit-threshold 2.5
```

### 4) Chạy Unit Test tự động
```bash
cd phan2-tool
go test -v ./...
```

## 3. Các giả định kỹ thuật, Ngưỡng đánh giá & Xử lý ngoại lệ

### 3.1. Công thức tính tải và Ngưỡng quá tải (Overload Thresholds)
Chỉ các VM đang ở trạng thái `running` mới tiêu thụ CPU và RAM thực tế:
- **CPU thực dùng của 1 VM (cores):** `vcpus * cpu_usage` (với `cpu_usage` thuộc `[0, 1]`).
- **Tỷ lệ CPU thực dùng của Node (%):** `(Tổng CPU thực dùng các VM running / cpu_cores) * 100`.
- **Tỷ lệ RAM thực dùng của Node (%):** `(Tổng mem_used các VM running / mem_total) * 100`.
- **Tỷ lệ Overcommit vCPU:** Thống kê đồng thời tỷ lệ của các VM đang chạy (`vcpus_running / cpu_cores`) và tổng toàn bộ VM (`vcpus_total / cpu_cores`).

Một node `online` bị đánh dấu là **QUÁ TẢI (OVERLOADED)** khi vượt ít nhất một trong 3 ngưỡng mặc định:
1. **CPU thực dùng > 80.0%**
2. **RAM thực dùng > 80.0%**
3. **vCPU Overcommit (Running VMs) > 2.5x** (250% số core vật lý).

### 3.2. Các bản ghi bất thường phát hiện trong `cluster-pve-hcm-c3.json`
1. **Node `hcm-c3-n04` (`status: "offline"`):** Đang bảo trì thay RAM từ 25/10 -> loại khỏi tập node đích nhận VM.
2. **Node `hcm-c3-n06` (`mem_total: 262144` bytes ~ 256 KiB):** Nhỏ hơn 1 GiB và mâu thuẫn với 133.1 GiB RAM đang dùng bởi 12 VM trên node này. Mặc định tool gắn cờ `INVALID_HARDWARE`, in cảnh báo và khóa chiều migrate VM mới vào `n06` (hỗ trợ thêm cờ `-fix-unit-anomaly` nếu xác nhận đây là lỗi đơn vị KiB tương đương 256 GiB).
3. **3 VM (`2067`, `2073`, `2065`) trên `hcm-c3-n02` có `cpu_usage` là `31.0`, `98.0`, `20.8`:** Vượt thang `[0..1]` do xuất nhầm theo đơn vị phần trăm (`%`). Tool tự động chia `100` để chuẩn hóa về `0.310`, `0.980`, `0.208` và ghi cảnh báo.
4. **Trùng lặp `vmid = 2001` (`gitlab-453`):** Xuất hiện ở cả `hcm-c3-n01` và `hcm-c3-n02`. Tool giữ bản ghi đầu tiên (`hcm-c3-n01`) và bỏ qua bản ghi trùng trên `hcm-c3-n02`.
5. **VM `2199` (`wp-host-377` trên `hcm-c3-n05`) có `mem_used: null`:** Áp dụng cơ chế fail-safe bảo toàn cho VM đang `running`, gán `mem_used = mem_max` (`50.0 GiB`).
6. **VM `2390` (`vps-nvme-512`) trỏ tới node `hcm-c3-n07`:** Node không tồn tại trong danh sách `nodes[]` -> ghi cảnh báo và bỏ qua.

## 4. Ví dụ Output thực tế

```text
============================================================================================================
 BÁO CÁO PHÂN TÍCH TẢI CLUSTER PROXMOX VE: pve-hcm-c3 (Snapshot: 2026-10-27T09:00:00+07:00)
 Ngưỡng quá tải: CPU > 80% | RAM > 80% | vCPU Overcommit (Running) > 2.5x
============================================================================================================

[1] CẢNH BÁO DỮ LIỆU BẤT THƯỜNG (DATA ANOMALIES & SANITIZATION):
  1. [Node hcm-c3-n04] Node đang ở trạng thái 'offline' (ghi chú: bảo trì thay RAM từ 25/10) -> loại khỏi danh sách node đích nhận live-migrate.
  2. [Node hcm-c3-n06] 'mem_total'=262144 bytes (256 KiB < 1 GiB) bất thường so với tổng RAM các VM trên node; đã gắn cờ INVALID_HARDWARE và khóa chiều nhận VM.
  3. [VM 2067 (wp-host-992)] 'cpu_usage'=31.0 vượt thang [0..1] (ghi nhầm theo phần trăm %) -> đã chuẩn hóa về 0.310.
  4. [VM 2073 (vps-nvme-430)] 'cpu_usage'=98.0 vượt thang [0..1] (ghi nhầm theo phần trăm %) -> đã chuẩn hóa về 0.980.
  5. [VM 2065 (cloud-std-682)] 'cpu_usage'=20.8 vượt thang [0..1] (ghi nhầm theo phần trăm %) -> đã chuẩn hóa về 0.208.
  6. [VM 2001 (gitlab-453)] Trùng lặp vmid=%d trên node 'hcm-c3-n02' (đã tồn tại trên 'hcm-c3-n01'), bỏ qua bản ghi trùng.
  7. [VM 2199 (wp-host-377)] 'mem_used' bị null trên node 'hcm-c3-n05' -> fallback an toàn bằng 'mem_max' (50.0 GiB).
  8. [VM 2390 (vps-nvme-512)] Tham chiếu tới node 'hcm-c3-n07' không tồn tại trong danh sách nodes, đã bỏ qua.

[2] MỨC TẢI CÁC NODE TRƯỚC KHI MIGRATE (CURRENT STATE):
Node       | Status  | VMs(Run/All) | CPU Thực Dùng    | RAM Thực Dùng         | vCPU Overcommit (Run/All) | Đánh Giá Trạng Thái
----       | ------  | ------------ | -------------    | -------------         | ------------------------- | -------------------
hcm-c3-n01 | online  | 14/15        | 54.99/64c (85.9%)| 476.2/512G (93.0%)    | 110/64 (1.72x | 1.97x)    | [!] OVERLOADED: CPU>80% (85.9%), RAM>80% (93.0%)
hcm-c3-n02 | online  | 13/13        | 25.01/64c (39.1%)| 281.6/512G (55.0%)    | 80/64 (1.25x | 1.25x)     | [OK] HEALTHY
hcm-c3-n03 | online  | 11/12        | 18.24/48c (38.0%)| 180.5/384G (47.0%)    | 80/48 (1.67x | 2.00x)     | [OK] HEALTHY
hcm-c3-n04 | offline | 0/0          | 0.00/48c (0.0%)  | 0.0/384G (0.0%)       | 0/48 (0.00x | 0.00x)      | [-] OFFLINE (bảo trì thay RAM từ 25/10)
hcm-c3-n05 | online  | 12/13        | 50.51/64c (78.9%)| 417.9/512G (81.6%)    | 72/64 (1.12x | 1.38x)     | [!] OVERLOADED: RAM>80% (81.6%)
hcm-c3-n06 | online  | 12/12        | 9.59/32c (30.0%) | 133.1G / 262144B (ERR)| 94/32 (2.94x | 2.94x)     | [!] OVERLOADED / INVALID_HW: vCPU_OVERCOMMIT>2.5x (2.94x)

[3] KẾ HOẠCH ĐỀ XUẤT LIVE-MIGRATE VM (MIGRATION PLAN):
Bước | VMID | Tên VM         | Từ Node    | Sang Node  | vCPU | CPU Thực | RAM Thực | Lý Do Chuyển                     | Lệnh Gợi Ý (Proxmox CLI)
---- | ---- | ------         | -------    | ---------  | ---- | -------- | -------- | ------------                     | ------------------------
1    | 2054 | cloud-std-657  | hcm-c3-n01 | hcm-c3-n02 | 8    | 7.58c    | 62.0 GiB | CPU>80% (85.9%), RAM>80% (93.0%) | qm migrate 2054 hcm-c3-n02 --online
2    | 2246 | wp-host-806    | hcm-c3-n05 | hcm-c3-n03 | 8    | 7.13c    | 51.8 GiB | RAM>80% (81.6%)                  | qm migrate 2246 hcm-c3-n03 --online
3    | 2057 | k8s-worker-295 | hcm-c3-n01 | hcm-c3-n03 | 8    | 7.38c    | 60.3 GiB | RAM>80% (80.9%)                  | qm migrate 2057 hcm-c3-n03 --online
4    | 2292 | k8s-worker-699 | hcm-c3-n06 | hcm-c3-n03 | 16   | 0.38c    | 5.4 GiB  | vCPU_OVERCOMMIT>2.5x (2.94x)     | qm migrate 2292 hcm-c3-n03 --online

[4] DỰ KIẾN MỨC TẢI CÁC NODE SAU KHI MIGRATE (PROJECTED STATE):
Node       | Status  | VMs(Run/All) | CPU Thực Dùng    | RAM Thực Dùng         | vCPU Overcommit (Run/All) | Đánh Giá Trạng Thái
----       | ------  | ------------ | -------------    | -------------         | ------------------------- | -------------------
hcm-c3-n01 | online  | 12/13        | 40.03/64c (62.6%)| 353.8/512G (69.1%)    | 94/64 (1.47x | 1.72x)     | [OK] HEALTHY
hcm-c3-n02 | online  | 14/14        | 32.59/64c (50.9%)| 343.6/512G (67.1%)    | 88/64 (1.38x | 1.38x)     | [OK] HEALTHY
hcm-c3-n03 | online  | 14/15        | 33.13/48c (69.0%)| 298.0/384G (77.6%)    | 112/48 (2.33x | 2.67x)    | [OK] HEALTHY
hcm-c3-n04 | offline | 0/0          | 0.00/48c (0.0%)  | 0.0/384G (0.0%)       | 0/48 (0.00x | 0.00x)      | [-] OFFLINE (bảo trì thay RAM từ 25/10)
hcm-c3-n05 | online  | 11/12        | 43.38/64c (67.8%)| 366.1/512G (71.5%)    | 64/64 (1.00x | 1.25x)     | [OK] HEALTHY
hcm-c3-n06 | online  | 11/11        | 9.21/32c (28.8%) | 127.7G / 262144B (ERR)| 78/32 (2.44x | 2.44x)     | [?] INVALID_HW (mem_total lỗi)
```
