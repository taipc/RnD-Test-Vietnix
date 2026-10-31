# Proxmox VE Cluster Load Analyzer & Live-Migration Planner (Go Edition)

Công cụ CLI viết bằng **Go (Golang)** dùng để giám sát mức tải của cụm Proxmox VE từ file snapshot JSON. Tool tự động phát hiện và làm sạch dữ liệu bất thường (data sanitization), đánh dấu các node quá tải, và sử dụng thuật toán **Greedy Best-Fit** để đề xuất kế hoạch `live-migrate` VM nhằm cân bằng tài nguyên toàn cụm một cách an toàn.

## 1. Yêu cầu hệ thống & Cấu trúc thư mục

- **Ngôn ngữ:** Go >= 1.20. Tool chỉ sử dụng Go Standard Library (`encoding/json`, `flag`, `text/tabwriter`, `testing`, `math`), hoàn toàn không phụ thuộc vào thư viện bên ngoài (Zero Dependencies).
- **Cấu trúc thư mục:**
  
```text
phan2-tool/
  go.mod          # Khai báo Go module
  main.go         # Mã nguồn chính (Parser, Sanitizer, Migration Planner, CLI Formatter)
  main_test.go    # Bộ Table-Driven Unit Test (Coverage ~88%)
  README.md       # Tài liệu hướng dẫn sử dụng
```

## 2. Hướng dẫn Cài đặt & Cách chạy

### Cài đặt / Biên dịch
Mở terminal tại thư mục `phan2-tool/` và chạy lệnh sau để build ra file thực thi:

```bash
# Trên Linux/macOS
go build -o pve-analyzer main.go

# Trên Windows
go build -o pve-analyzer.exe main.go
```

### Cách chạy Tool

**1. Hiển thị dạng Bảng trực quan trên Terminal (Mặc định)**
```bash
./pve-analyzer -f cluster-pve-hcm-c3.json -o table
```

**2. Xuất định dạng JSON và lưu ra file (Tích hợp CI/CD, Ansible)**
Cảnh báo lỗi (Warnings) sẽ được in ra màn hình (stderr), trong khi báo cáo JSON sạch sẽ được lưu vào file `ketqua-migrate.json`.
```bash
./pve-analyzer -f cluster-pve-hcm-c3.json -o json -out ketqua-migrate.json
```

**3. Tùy chỉnh các ngưỡng đánh giá quá tải**
```bash
./pve-analyzer -f cluster-pve-hcm-c3.json -cpu-threshold 75 -mem-threshold 85 -overcommit-threshold 3.0
```

**4. Chạy bộ Unit Test tự động nghiệm thu**
```bash
go test -v -cover
```

## 3. Các giả định kỹ thuật & Ngưỡng đánh giá (Thresholds)

### 3.1. Công thức tính mức tải
Công cụ áp dụng nguyên tắc: **Chỉ các VM đang chạy (`status: "running"`) mới tiêu thụ CPU và RAM thực tế**.
- **CPU VM (cores):** `vcpus * cpu_usage` (với `cpu_usage` chuẩn hóa về `[0..1]`).
- **Node CPU Usage (%):** `(Tổng CPU VM running / cpu_cores) * 100`.
- **Node RAM Usage (%):** `(Tổng mem_used VM running / mem_total) * 100`.
- **vCPU Overcommit Ratio:** `Tổng vcpus của VM running / cpu_cores`.

### 3.2. Ngưỡng báo động (Overload Thresholds)
Một node được đánh giá là **OVERLOADED** nếu vi phạm ít nhất 1 trong 3 ngưỡng (có thể tùy chỉnh qua arguments):
1. **CPU thực dùng > 80.0%**
2. **RAM thực dùng > 80.0%**
3. **vCPU Overcommit (Running VMs) > 2.5x** (Vượt quá 250% số core vật lý).

### 3.3. Xử lý bẫy dữ liệu & Ngoại lệ an toàn (Fail-Safe)
Tool được thiết kế "phòng thủ" chặt chẽ, không bao giờ bị crash trước dữ liệu bẩn:
- **Lỗi thiếu thông số RAM (`mem_used: null` hoặc âm):** Fallback an toàn, tự động dùng `mem_max` nếu VM đang chạy.
- **Node lỗi RAM (< 1 GiB):** Gắn cờ `INVALID_HARDWARE`, khóa không cho phép migrate thêm VM mới vào node này để tránh sập hệ thống (ví dụ: Node n06 báo có 256KiB RAM nhưng gánh 12 VMs). Cờ `-fix-unit-anomaly` hỗ trợ tự quy đổi nếu xác nhận đây là lỗi đơn vị.
- **Lỗi thang đo CPU:** Nếu `cpu_usage` > 1.0 (ví dụ `98.0`), tự động nhận diện đây là phần trăm (%) và chia 100 để đưa về chuẩn `0.98`.
- **Dữ liệu mồ côi / Trùng lặp:** Bỏ qua các VM trỏ tới Node ma không tồn tại, tự động lọc VM bị trùng `vmid`.
- **Node Offline:** Nhận diện trạng thái offline, tuyệt đối không chọn làm đích đến cho tiến trình Live-Migration.

## 4. Ví dụ Output thực tế (Dạng Table)

```text
============================================================================================================
 BÁO CÁO PHÂN TÍCH TẢI CLUSTER PROXMOX VE: pve-hcm-c3 (Snapshot: 2026-10-27T09:00:00+07:00)
 Ngưỡng quá tải: CPU > 80% | RAM > 80% | vCPU Overcommit (Running) > 2.5x
============================================================================================================

[1] CẢNH BÁO DỮ LIỆU BẤT THƯỜNG (DATA ANOMALIES & SANITIZATION):
  1. [Node hcm-c3-n04] Node đang ở trạng thái 'offline' -> loại khỏi danh sách node đích nhận live-migrate.
  2. [Node hcm-c3-n06] 'mem_total' bất thường (<1 GiB); đã gắn cờ INVALID_HARDWARE và khóa chiều nhận VM.
  3. [VM 2073 (vps-nvme-430)] 'cpu_usage'=98.0 vượt thang [0..1] -> đã chuẩn hóa về 0.980.
  4. [VM 2199 (wp-host-377)] 'mem_used' bị null -> fallback an toàn bằng 'mem_max' (50.0 GiB).

[2] MỨC TẢI CÁC NODE TRƯỚC KHI MIGRATE (CURRENT STATE):
Node       Status  VMs(Run/All) CPU Thực Dùng     RAM Thực Dùng          vCPU Overcommit       Đánh Giá
----       ------  ------------ -------------     -------------          ---------------       --------
hcm-c3-n01 online  14/15        54.99/64c (85.9%) 476.2/512G (93.0%)     110/64 (1.72x)        [!] OVERLOADED: CPU>80%, RAM>80%
hcm-c3-n02 online  13/13        25.01/64c (39.1%) 281.6/512G (55.0%)     80/64 (1.25x)         [OK] HEALTHY
hcm-c3-n03 online  11/12        18.24/48c (38.0%) 180.5/384G (47.0%)     80/48 (1.67x)         [OK] HEALTHY

[3] KẾ HOẠCH ĐỀ XUẤT LIVE-MIGRATE VM (MIGRATION PLAN):
Bước VMID Tên VM         Từ Node    Sang Node  vCPU CPU Thực RAM Thực Lý Do Chuyển      Lệnh Gợi Ý
---- ---- ------         -------    ---------  ---- -------- -------- ------------      ----------
1    2054 cloud-std-657  hcm-c3-n01 hcm-c3-n02 8    7.58c    62.0 GiB CPU>80%, RAM>80%  qm migrate 2054 hcm-c3-n02 --online
2    2246 wp-host-806    hcm-c3-n05 hcm-c3-n03 8    7.13c    51.8 GiB RAM>80%           qm migrate 2246 hcm-c3-n03 --online

[4] DỰ KIẾN MỨC TẢI CÁC NODE SAU KHI MIGRATE (PROJECTED STATE):
Node       Status  VMs(Run/All) CPU Thực Dùng     RAM Thực Dùng          vCPU Overcommit       Đánh Giá
----       ------  ------------ -------------     -------------          ---------------       --------
hcm-c3-n01 online  12/13        40.03/64c (62.6%) 353.8/512G (69.1%)     94/64 (1.47x)         [OK] HEALTHY
hcm-c3-n02 online  14/14        32.59/64c (50.9%) 343.6/512G (67.1%)     88/64 (1.38x)         [OK] HEALTHY
```