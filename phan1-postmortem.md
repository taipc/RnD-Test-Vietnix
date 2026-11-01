# Báo Cáo Postmortem: Sự cố gián đoạn dịch vụ trên web-hcm-07 (Nginx 502/504 & MySQL Crash)

**Ngày xảy ra sự cố:** 20/10/2026
**Hệ thống ảnh hưởng:** Máy chủ `web-hcm-07` (Nginx, PHP-FPM, MySQL)
**Thời gian downtime:** ~45 phút (01:30 – 02:15)
**Mức độ nghiêm trọng (Severity):** High / SEV-1

---

## 1. Tóm tắt sự cố (Executive Summary)
Vào rạng sáng ngày 20/10/2026, máy chủ `web-hcm-07` gặp sự cố gián đoạn dịch vụ toàn phần. Người dùng truy cập website nhận mã lỗi HTTP 502 Bad Gateway và 504 Gateway Timeout. Nguyên nhân gốc rễ (Root Cause) được xác định là do phân vùng ổ đĩa (Disk Space) bị đầy 100% xuất phát từ tiến trình `logrotate` chạy trong `cron.daily` cấu hình sai, không giải phóng được dung lượng log cũ. Hậu quả là MySQL không thể ghi data/binlog và bị crash, kéo theo PHP-FPM cạn kiệt worker pool, dẫn đến Nginx không thể đẩy request tới backend.

## 2. Diễn biến sự cố (Timeline)
*(Các mốc thời gian theo giờ hệ thống UTC+7)*

* **00:00:** Cronjob hệ thống kích hoạt chạy tác vụ `/etc/cron.daily/logrotate`.
* **00:15:** Tiến trình `logrotate` thất bại trong việc xoay vòng và nén log do sai cấu hình (các file log khổng lồ cũ không được xóa, đồng thời Nginx vẫn giữ file descriptor ghi tiếp vào file cũ).
* **01:30:** Lượng access/error log và MySQL binlog phát sinh thêm làm phân vùng `/` (chứa `/var`) chạm ngưỡng 100% disk usage. Hệ thống giám sát bắn cảnh báo Disk Space Critical.
* **01:32:** Tiến trình `mysqld` thất bại khi cố gắng flush dữ liệu và ghi binlog xuống đĩa (`No space left on device`), dẫn đến crash. Database ngừng phản hồi.
* **01:35:** Các tiến trình `php-fpm` bị treo ở trạng thái chờ kết nối/query tới MySQL, số lượng worker bị chiếm dụng tăng vọt cho đến khi chạm trần (`pm.max_children reached`).
* **01:38:** Nginx không thể giao tiếp với upstream `php-fpm` qua Unix socket/TCP port, bắt đầu trả về mã lỗi 502 Bad Gateway và 504 Gateway Timeout cho người dùng. Cảnh báo HTTP 5xx tăng đột biến.
* **01:45:** Kỹ sư On-call tiếp nhận cảnh báo và SSH vào máy chủ điều tra.
* **01:50:** Kỹ sư xác định nguyên nhân gốc do đầy ổ đĩa 100% thông qua lệnh `df -h` và `lsof +L1`.
* **01:55:** Kỹ sư tiến hành dọn dẹp khẩn cấp: xóa các bản lưu log `.gz` cũ và truncate trực tiếp các file log đang mở để giải phóng dung lượng đĩa tức thời.
* **02:00:** Khởi động lại tuần tự các dịch vụ: MySQL, PHP-FPM và Nginx.
* **02:15:** Kiểm tra InnoDB crash recovery hoàn tất, theo dõi các chỉ số tải và xác nhận dịch vụ hoạt động ổn định trở lại. Đóng sự cố.

## 3. Nguyên nhân gốc rễ (Root Cause)
1. **Cạn kiệt dung lượng lưu trữ:** Phân vùng hệ thống đầy 100% (`No space left on device`) khiến các tiến trình không thể thực hiện thao tác ghi I/O.
2. **Lỗi cấu hình Logrotate & Phân vùng:** 
   - File cấu hình `/etc/logrotate.d/nginx` bị lỗi cú pháp hoặc thiếu cờ `compress` / `maxsize`, hoặc thiếu script `postrotate` để gửi tín hiệu `USR1` cho Nginx mở lại file log mới, làm dung lượng không được giải phóng qua nhiều ngày.
   - Thư mục `/var/log` và `/var/lib/mysql` nằm chung trên một phân vùng gốc (`/`) mà không được tách riêng (LVM/Mount point độc lập), dẫn đến việc log phình to làm chết luôn cả Database.
3. **Phản ứng dây chuyền (Cascading Failure):** 
   - MySQL crash do không ghi được Redo Log / Binlog.
   - PHP-FPM thiếu cấu hình `request_terminate_timeout` chặt chẽ và DB connection timeout quá dài, dẫn đến treo toàn bộ worker pool.
   - Nginx báo lỗi 502/504 do upstream PHP-FPM từ chối kết nối hoặc quá thời gian phản hồi.

## 4. Ảnh hưởng (Impact)
* **Dịch vụ:** Toàn bộ các website trên máy chủ `web-hcm-07` bị gián đoạn truy cập hoàn toàn (Downtime 100%) trong khoảng 45 phút.
* **Dữ liệu:** Các giao dịch (transactions) đang thực thi dở dang tại thời điểm 01:32 bị rollback. Cơ chế tự phục hồi (Crash Recovery) của InnoDB đã đảm bảo tính toàn vẹn dữ liệu khi MySQL khởi động lại, không xảy ra hỏng bảng (corrupt table).

## 5. Hành động khắc phục tức thời (Resolution / Mitigation)
1. Truy cập SSH vào máy chủ `web-hcm-07`.
2. Kiểm tra dung lượng phân vùng và xác định thư mục chiếm dụng bằng lệnh `df -h` và `du -sh /var/log/*`.
3. Xóa các file nén log cũ đã lưu trữ lâu ngày: `rm -f /var/log/nginx/*.gz`.
4. Làm rỗng (truncate) các file log đang được tiến trình mở (tuyệt đối không dùng `rm` trực tiếp để tránh lỗi file bị xóa nhưng tiến trình vẫn giữ inode gây chiếm dụng đĩa ngầm):
   ```bash
   truncate -s 0 /var/log/nginx/access.log
   truncate -s 0 /var/log/nginx/error.log
   ```
5. Khởi động lại chuỗi dịch vụ theo thứ tự phụ thuộc (Database -> App -> Web Server):
   ```bash
   systemctl start mysql
   systemctl restart php8.x-fpm
   systemctl restart nginx
   ```

## 6. Bài học kinh nghiệm & Hành động phòng ngừa (Action Items)

| Hạng mục | Hành động cụ thể | Mức ưu tiên | Thời hạn |
|---|---|---|---|
| **Log Management** | Rà soát và sửa lại `/etc/logrotate.d/nginx`: bổ sung giới hạn `maxsize 500M`, nén `compress`, giữ tối đa `rotate 7` và kiểm thử lại bằng lệnh `logrotate -d /etc/logrotate.conf`. | P0 (Khẩn cấp) | Hoàn thành trong ngày |
| **Database Config** | Thiết lập tham số `binlog_expire_logs_seconds = 259200` (3 ngày) trong `my.cnf` để MySQL tự động dọn dẹp binlog cũ. | P1 (Cao) | 1 - 2 ngày |
| **Monitoring & Alert** | Tinh chỉnh lại ngưỡng cảnh báo trên Prometheus/Zabbix: Cảnh báo sớm (Warning) khi Disk Usage đạt **75%** và báo động đỏ (Critical) ở mức **85%**, không đợi đến 100% mới báo. | P1 (Cao) | Trong tuần |
| **Service Isolation** | Cấu hình giới hạn `request_terminate_timeout = 30s` trong PHP-FPM và timeout kết nối MySQL ngắn lại (3-5s) để giải phóng nhanh worker khi DB gặp sự cố (Fail-fast). | P2 (Trung bình) | Trong tuần |
| **Infrastructure** | Chuẩn hóa lại template cài đặt Server: tách riêng phân vùng `/var/log` và `/var/lib/mysql` trên các Logical Volume (LVM) độc lập để lỗi tràn log không làm sập Database. | P2 (Trung bình) | Kế hoạch quý |