# Bài Test Kỹ Thuật: R&D Engineer - Vietnix

Repository này lưu trữ toàn bộ mã nguồn và tài liệu giải quyết bài test kỹ thuật cho vị trí R&D Engineer.

## 📁 Cấu trúc Repository

- **`phan1-postmortem.md`**: Báo cáo sự cố (Postmortem) xử lý lỗi Nginx 502/504 và sự cố MySQL Crash trên Node web-hcm-07.
- **`phan2-tool/`**: Công cụ CLI (Golang) phân tích tải và lập kế hoạch Live-Migrate tự động cho cụm máy chủ Proxmox VE.
  - 👉 *Xem tài liệu kỹ thuật, hướng dẫn chạy và các giả định thuật toán chi tiết tại:* [`phan2-tool/README.md`](./phan2-tool/README.md)
- **`phan3-nghien-cuu.md`**: Báo cáo đánh giá kiến trúc, hiệu năng và đề xuất giải pháp Object Storage S3 quy mô nhỏ (MinIO vs Ceph RGW).
- **`phan4-nhat-ky.md`**: Nhật ký làm bài, ghi nhận tổng thời gian, các rào cản kỹ thuật và quy trình nghiệm thu.

## 💡 Các giả định cốt lõi (Core Assumptions)

Trong quá trình thiết kế giải pháp, các giả định sau đã được áp dụng để đảm bảo tính an toàn và thực tiễn:
1. **Quản trị tài nguyên (Phần 2):** Việc tính toán tải của Cluster chỉ áp dụng trên các VM đang ở trạng thái `running`. Các bất thường từ snapshot JSON (như RAM node < 1GB, thiếu mem_used, sai thang đo CPU) được xác định là "bẫy dữ liệu" và công cụ có cơ chế Fail-Safe tự động cô lập thay vì để xảy ra lỗi crash.
2. **Quy hoạch kiến trúc (Phần 3):** Giả định cụm Storage 50TB (3-4 servers) sẽ được thiết kế chuyên biệt (Dedicated) cho giao thức S3, không mang mục đích chia sẻ tài nguyên chung để cấp phát Block Storage (RBD) cho các hệ thống máy ảo khác.
