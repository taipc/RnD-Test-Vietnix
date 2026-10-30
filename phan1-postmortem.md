# Báo Cáo Postmortem: Sự cố gián đoạn dịch vụ trên web-hcm-07 (Nginx 502/504 & MySQL Crash)

**Ngày xảy ra sự cố:** 20/10/2026
**Hệ thống ảnh hưởng:** Máy chủ `web-hcm-07` (Nginx, PHP-FPM, MySQL)
**Thời gian downtime:** ~45 phút
**Mức độ nghiêm trọng (Severity):** High / SEV-1

---

## 1. Tóm tắt sự cố (Executive Summary)
Vào rạng sáng ngày 20/10/2026, máy chủ `web-hcm-07` gặp sự cố gián đoạn dịch vụ toàn phần. Người dùng truy cập website nhận mã lỗi HTTP 502 Bad Gateway và 504 Gateway Timeout. Nguyên nhân gốc rễ (Root Cause) được xác định là do phân vùng ổ đĩa (Disk Space) bị đầy 100% xuất phát từ tiến trình `logrotate` chạy trong `cron.daily` không dọn dẹp log cũ đúng cách. Hậu quả là MySQL không thể ghi data/binlog và bị crash, kéo theo PHP-FPM cạn kiệt worker pool, dẫn đến Nginx không thể đẩy request tới backend.

## 2. Diễn biến sự cố (Timeline)
*(Các mốc thời gian mang tính tương đối trong chu kỳ sự cố)*

* **00:00:** Cronjob hệ thống kích hoạt chạy tác vụ `/etc/cron.daily/logrotate`.
* **00:15:** Cấu hình logrotate bị lỗi khiến các file log (access log, error log) không được nén hoặc xoá, phình to nhanh chóng.
* **01:30:** Cảnh báo (Alert) ghi nhận phân vùng `/` (hoặc `/var`) đạt mức 100% disk usage.
* **01:32:** Tiến trình `mysqld` thất bại khi cố gắng ghi vào đĩa, tiến trình bị killed/crashed. Database ngừng phản hồi.
* **01:35:** Các tiến trình `php-fpm` kẹt ở trạng thái chờ kết nối DB, số lượng worker tăng vọt cho đến khi cạn kiệt (max_children reached).
* **01:38:** Nginx không thể giao tiếp với upstream `php-fpm` qua socket/port, bắt đầu trả về mã lỗi 502 Bad Gateway và 504 Gateway Timeout cho người dùng. Alert HTTP 5xx tăng đột biến.
* **01:45:** Kỹ sư On-call tiếp nhận sự cố và bắt đầu điều tra.
* **01:50:** Kỹ sư xác định nguyên nhân do đầy ổ đĩa (thông qua lệnh `df -h`).
* **01:55:** Kỹ sư tiến hành dọn dẹp thủ công (xóa các file log `.gz` cũ và truncate các file log hiện tại) để giải phóng không gian lưu trữ.
* **02:00:** Khởi động lại các dịch vụ: `systemctl restart mysql php-fpm nginx`.
* **02:15:** Kiểm tra lại hệ thống (Service Healthcheck), dịch vụ hoạt động ổn định trở lại. Đóng luồng xử lý sự cố.

## 3. Nguyên nhân gốc rễ (Root Cause)
1. **Lỗi tiến trình cấp phát đĩa:** Dung lượng lưu trữ cạn kiệt (No space left on device) khiến hệ điều hành và các ứng dụng không thể thực hiện thao tác I/O.
2. **Lỗi cấu hình Logrotate:** Tác vụ dọn dẹp log tự động (`logrotate`) bị sai cấu hình (ví dụ: thiếu tham số `compress`, `missingok`, hoặc số vòng lặp `rotate` quá lớn), khiến log không được nén và giữ lại quá lâu.
3. **Phản ứng dây chuyền (Cascading Failure):** 
   - MySQL crash do không ghi được WAL/Binlog.
   - PHP-FPM treo do kết nối database bị gián đoạn, làm cạn kiệt worker pool.
   - Nginx báo lỗi 502/504 do timeout hoặc connection refused từ PHP-FPM.

## 4. Ảnh hưởng (Impact)
* **Dịch vụ:** Toàn bộ website trên `web-hcm-07` không thể truy cập (Downtime 100%) trong khoảng 45 phút.
* **Dữ liệu:** Có nguy cơ mất mát các giao dịch (transactions) chưa kịp ghi xuống đĩa tại thời điểm MySQL bị crash (tuy nhiên bước kiểm tra sau phục hồi cho thấy dữ liệu an toàn).

## 5. Hành động khắc phục tức thời (Resolution / Mitigation)
1. Truy cập SSH vào máy chủ.
2. Sử dụng lệnh `df -h` và `du -sh /*` để xác định phân vùng bị đầy (`/var/log`).
3. Xóa các file nén log cũ: `rm -rf /var/log/nginx/*.gz`.
4. Truncate các file log đang hoạt động (không dùng `rm` để tránh treo inode): `> /var/log/nginx/access.log`.
5. Khởi động lại chuỗi dịch vụ:
   ```bash
   systemctl start mysql
   systemctl restart php8.x-fpm
   systemctl restart nginx