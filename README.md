# Bài Test Kỹ Thuật: R&D Engineer (Infrastructure & Automation) - Vietnix

Repository này chứa toàn bộ tài liệu và mã nguồn giải quyết bài test kỹ thuật cho vị trí R&D Engineer.

## 📁 Cấu trúc Repository

- `phan1-postmortem.md`: Báo cáo sự cố (Postmortem) web-hcm-07 (Nginx 502/504 & MySQL Crash).
- `phan2-tool/`: Thư mục chứa mã nguồn CLI Tool viết bằng Go (Golang) để phân tích tải Proxmox.
- `phan3-nghien-cuu.md`: Báo cáo nghiên cứu và so sánh giải pháp Object Storage (MinIO vs Ceph RGW).
- `phan4-nhat-ky.md`: Nhật ký làm bài (Time log/Work log).

---

## 🛠 Phần 2: Proxmox VE Cluster Load Analyzer & Live-Migration Planner

### 1. Tổng quan
Đây là công cụ CLI viết bằng **Go (Golang)** không sử dụng thư viện ngoài (chỉ dùng Standard Library). Công cụ tự động phân tích file snapshot JSON của cụm Proxmox, làm sạch các bẫy dữ liệu (data sanitization), đánh giá mức tải các Node vật lý và đề xuất kế hoạch `live-migrate` VM để cân bằng tài nguyên tự động.

### 2. Hướng dẫn chạy Tool

**Yêu cầu:** Hệ thống đã cài đặt Go (>= 1.20).

Di chuyển vào thư mục chứa mã nguồn:
```bash
cd phan2-tool
