# Phần 3: Nghiên cứu phương án Object Storage S3 quy mô nhỏ

**Ngữ cảnh triển khai:** Dịch vụ S3 cho doanh nghiệp, quy mô ~50TB, hạ tầng ban đầu 3-4 servers, yêu cầu chịu lỗi (Fault Tolerance) tối thiểu 1 server.

Dựa trên các giới hạn phần cứng này, dưới đây là phân tích so sánh giữa **MinIO** và **Ceph RGW (RADOS Gateway)**.

## 1. Tiêu chí so sánh và Dẫn chứng kỹ thuật

### A. Kiến trúc & Tiêu hao tài nguyên (Dẫn chứng Official Docs)
*   **Ceph RGW:** Được viết chủ yếu bằng C++, kiến trúc phân tán đa tầng. Chạy S3 yêu cầu tối thiểu bộ daemon: MON, MGR, OSD và RGW. Theo tài liệu chính thức [Ceph Hardware Recommendations](https://docs.ceph.com/en/latest/start/hardware-recommendations/), mỗi daemon OSD quản lý 1 ổ cứng cần tối thiểu 1-2GB RAM cho mỗi 1TB dữ liệu (chưa tính RAM cho HĐH và RGW). Với cụm 50TB, nội việc duy trì metadata cho OSD đã ngốn lượng RAM quá lớn so với quy mô 3-4 node.
*   **MinIO:** Được viết bằng Golang, kiến trúc nguyên khối (Single binary), thiết kế Native-S3. Không dùng cơ sở dữ liệu metadata bên ngoài (metadata ghi cùng object). Tối ưu hóa cực tốt cho quy mô nhỏ, khởi động tốn rất ít RAM (theo [MinIO Architecture](https://min.io/docs/minio/linux/operations/concepts/architecture.html)).

### B. Cơ chế bảo vệ & Chịu lỗi (Tolerate 1 Server)
*   **MinIO:** Với 4 node, MinIO dùng Erasure Coding (cấu hình `EC:2`). Thuật toán EC của MinIO cho phép hệ thống chịu lỗi mất hoàn toàn 1 node (hoặc 2 ổ đĩa) mà vẫn duy trì Read/Write bình thường (tham khảo [MinIO Erasure Coding](https://min.io/docs/minio/linux/operations/concepts/erasure-coding.html)). 
*   **Ceph RGW:** Thuật toán CRUSH map của Ceph cực kỳ mạnh mẽ cho hàng trăm node, nhưng trên cụm 3-4 node, việc cấu hình Erasure Coding rất khó cân bằng (unbalanced PG), rủi ro phân mảnh dữ liệu cao. Giải pháp an toàn nhất trên 3 node Ceph thường là dùng Replication (size=3), dẫn đến Storage Efficiency chỉ đạt 33% (quá lãng phí so với mức 50-75% của EC trên MinIO).

### C. Hiệu năng tính toán (Dẫn chứng Source Code & Benchmark)
*   **Về Source Code:** Mã nguồn MinIO khai thác thư viện mã nguồn mở [klauspost/reedsolomon](https://github.com/klauspost/reedsolomon) (Golang) tích hợp sẵn các tập lệnh SIMD (AVX-512, AVX2) trên CPU. Điều này giúp MinIO tính toán mã hóa Erasure Coding trực tiếp trên thanh ghi CPU với tốc độ cực cao, bypass các tầng block/file system trung gian.
*   **Về Benchmark uy tín:** Theo báo cáo [Performance & Benchmarks](https://min.io/product/performance) công bố trên trang chủ MinIO, khi chạy với ổ NVMe và mạng 100GbE, throughput của MinIO có thể bão hòa băng thông mạng (lên tới hàng trăm GB/s). Ngược lại, luồng I/O của Ceph RGW đi qua quá nhiều lớp dịch mã (`S3 API -> librgw -> librados -> RADOS -> BlueStore`), khiến độ trễ (latency) với các file nhỏ (KB) cao hơn đáng kể.

## 2. Khuyến nghị chốt & Đánh đổi (Trade-offs)

### Khuyến nghị: Chọn MinIO
Với bài toán 50TB và giới hạn 3-4 server, **MinIO là giải pháp vượt trội hoàn toàn**. Hạ tầng nhẹ, tận dụng 100% tài nguyên cho I/O thay vì nuôi các tiến trình quản lý cụm, và hỗ trợ tính khả dụng cao (chết 1 node vẫn chạy tốt) với Erasure Coding.

### Đánh giá rủi ro kỹ thuật (Trade-offs)
Chọn MinIO đồng nghĩa Vietnix phải chấp nhận các sự đánh đổi sau:
1.  **Tính cứng nhắc khi scale (Mở rộng):** Ceph cho phép scale-out bằng cách cắm thêm từng ổ cứng lẻ hoặc 1 node lẻ và tự động rebalance. Với MinIO, quản trị viên phải mở rộng bằng cách thêm hẳn một Server Pool mới (ví dụ phải thêm 1 cụm 4 node nữa) với cấu hình tương đương.
2.  **Khả năng đa mục đích:** Cụm MinIO chỉ phục vụ duy nhất giao thức S3. Nếu Vietnix muốn sau này dùng chính 4 server đó để cấp phát Block Storage (RBD) cho máy ảo Proxmox, MinIO sẽ bất lực. Lúc đó, Ceph mới là giải pháp "Một hạ tầng - Đa dịch vụ" đúng nghĩa.