# Báo Cáo Postmortem: Sự cố gián đoạn dịch vụ trên web-hcm-07 (Nginx 502/504 & MySQL Crash)

**Ngày xảy ra sự cố:** 20/10/2026
**Hệ thống ảnh hưởng:** Máy chủ `web-hcm-07` (Nginx, PHP-FPM, MySQL)
**Thời gian downtime:** ~45 phút (01:30 – 02:15)
**Mức độ nghiêm trọng (Severity):** High / SEV-1

---

## 1. Tóm tắt sự cố (Executive Summary)
Vào rạng sáng ngày 20/10/2026, máy chủ `web-hcm-07` gặp sự cố gián đoạn dịch vụ toàn phần. Người dùng truy cập website nhận mã lỗi HTTP 502 Bad Gateway và 504 Gateway Timeout. Nguyên nhân gốc rễ (Root Cause) được xác định là do phân vùng ổ đĩa (Disk Space) bị đầy 100% xuất phát từ tiến trình `logrotate` chạy trong `cron.daily` cấu hình sai, không giải phóng được dung lượng log cũ. Hậu quả là MySQL không thể ghi data/binlog và bị crash, kéo theo PHP-FPM cạn kiệt worker pool, dẫn đến Nginx không thể đẩy request tới backend.

## 2. Diễn biến sự cố (Timeline)
User hieunv sử dụng vim để sửa tập tin /etc/logrotate.d/nginx vào lúc :
- Oct 13 17:42:30 web-hcm-07 sudo:   hieunv : TTY=pts/0 ; PWD=/home/hieunv ; USER=root ; COMMAND=/usr/bin/vim /etc/logrotate.d/nginx
Sau đó tiền trình ghi log hàng ngày được kích hoạt:
- Oct 14 00:00:01 web-hcm-07 CRON[5198]: (root) CMD (test -x /usr/sbin/logrotate && /usr/sbin/logrotate /etc/logrotate.conf)
Ngay lập tức xảy ra lỗi: 
- Oct 14 00:00:02 web-hcm-07 logrotate[5199]: error: /etc/logrotate.d/nginx:3 unknown option 'dailly' -- ignoring line
Lỗi chính tả này đã ngăn hệ thống dọn dẹp log của Nginx. Trình logrotate liên tục báo lỗi và bỏ qua việc cắt/nén các tệp log trong thư mục /var/log/nginx/ kể từ ngày 14/10. Hậu quả là các tệp log không ngừng phình to theo thời gian từ :
- Oct 14 00:30:02 web-hcm-07 disk-check: OK / usage 57%
cho đến khi vắt kiệt hoàn toàn dung lượng ổ cứng (No space left on device) vào rạng sáng ngày 20/10:
- Oct 20 00:30:02 web-hcm-07 disk-check: WARN / usage 87%
Việc dịch vụ dọn dẹp báo lỗi liên tục mỗi đêm kết hợp với tốc độ tiêu hao ổ cứng tăng vọt (từ 57% lên 87% trong vỏn vẹn 6 ngày) là bằng chứng rõ ràng nhất cho thấy log Nginx chính là thủ phạm gây tràn ổ cứng.
Dẫn đến:
1. Tình trạng hết không gian lưu trữ này đã trực tiếp làm gián đoạn tiến trình sao lưu và khiến dịch vụ cơ sở dữ liệu MySQL bị treo:
   - 2026-10-20T01:33:04.118842+07:00 14 [ERROR] [MY-000035] [Server] Disk is full writing './binlog.000187' (OS errno 28 - No space left on device). Waiting for someone to free space... Retry in 60 secs. Further messages will be issued every 10 minutes.
2. Máy chủ web đang không thể ghi nhật ký truy cập vì đĩa cứng đã đầy.
   - 2026/10/20 01:32:59 [crit] 2104#0: *5532190 pwrite() "/var/lib/nginx/body/0000183421" failed (28: No space left on device), client: 113.161.40.12, server: shop.anhduongfood.vn, request: "POST /wp-admin/admin-ajax.php HTTP/1.1", host: "shop.anhduongfood.vn"
3. Mã nguồn website (PHP/WordPress) cảnh báo lỗi không thể khởi tạo phiên làm việc của người dùng do cạn kiệt dung lượng đĩa (No space left on device):
   - [20-Oct-2026 01:33:06] WARNING: [pool www] child 90311 said into stderr: "PHP Warning:  session_start(): open(/var/lib/php/sessions/sess_9f2c1d, O_RDWR) failed: No space left on device (28) in /home/site42/public_html/wp-includes/class-wp-session.php on line 112"

## 3. Nguyên nhân gốc rễ (Root Cause)
   - File cấu hình `/etc/logrotate.d/nginx` bị lỗi unknown option 'dailly', Lỗi chính tả này đã ngăn hệ thống dọn dẹp log của Nginx. Trình logrotate liên tục báo lỗi và bỏ qua việc cắt/nén các tệp log trong thư mục /var/log/nginx/ kể từ ngày 14/10. Hậu quả là các tệp log không ngừng phình to theo thời gian cho đến khi vắt kiệt hoàn toàn dung lượng ổ cứng (No space left on device) vào rạng sáng ngày 20/10.
   Có 5 thông tin đáng lưu ý:
   1. file log phình lên 37G (dòng số 10 trong file RnD-Test-Vietnix\phan1-su-co\snapshot\du-var.txt): 
   - /var/log/nginx/:
     -rw-r----- 1 www-data adm  37G Oct 20 01:33 access.log
   2. Trong code bash có đoạn if...then(dòng số 8-9 trong file RnD-Test-Vietnix\phan1-su-co\snapshot\backup-sites.sh):
   - "if tar -czf "$DEST/sites-$TODAY.tar.gz" /home 2>&1 | logger -t backup-sites; then
    SIZE=$(du -h "$DEST/sites-$TODAY.tar.gz" | cut -f1)"
   Giải thích logic trên : Script chạy lệnh tar -czf tạo bản backup ngày 20/10 trước, nếu thành công (if ... then) mới chạy lệnh rm -f để xóa bản backup ngày 19/10. Tức là vào lúc 01:00 sáng, ổ cứng luôn phải chịu tải gấp đôi dung lượng backup cùng một lúc.
   3. Thông qua log trong syslog ghi nhận việc backup diễn ra vào 1h sáng hàng ngày :
   - Oct 19 01:00:01 web-hcm-07 CRON[14495]: (root) CMD (/usr/local/bin/backup-sites.sh >> /var/log/backup-sites.log 2>&1)
   - Oct 20 01:00:01 web-hcm-07 CRON[16887]: (root) CMD (/usr/local/bin/backup-sites.sh >> /var/log/backup-sites.log 2>&1)
   4. Việc backups gặp error do dung lương ổ cứng chạm đáy 0 byte:
   - Oct 20 01:32:57 web-hcm-07 backup-sites: tar: /var/backups/sites-20261020.tar.gz: Wrote only 4096 of 10240 bytes
   5. Thông tin lưu trữ file backup có được từ file du-var.txt sau khi lỗi backup:
   - /var/backups/:
     -rw-r--r-- 1 root root  19G Oct 19 01:41 sites-20261019.tar.gz
     -rw-r--r-- 1 root root  16G Oct 20 01:32 sites-20261020.tar.gz
   **Tổng hợp các thông tin trên có thể thấy được 2 nguyên nhân gốc rễ :
   1. Không chỉ do file /var/log/nginx/access.log phình lên 37G , chính logic của script /usr/local/bin/backup-sites.sh cũng góp phần tiêu hao phần dung lượng trống cuối cùng. Thư mục /var/backups/ hiện ôm cứng 35G (19G của ngày 19/10 + 16G file hỏng của ngày 20/10), khóa chết hoàn toàn ổ đĩa khiến hệ thống không thể tự hồi phục dù tiến trình backup đã dừng.
   2. Thêm vào đó tình trạng hết không gian lưu trữ này đã trực tiếp làm gián đoạn tiến trình sao lưu và khiến dịch vụ cơ sở dữ liệu MySQL bị treo do máy chủ chỉ có duy nhất 1 phân vùng chính là /dev/sda2(df-h.txt): 
   -  Tue Oct 20 02:23:41 AM +07 2026 : /dev/sda2       118G  118G     0 100% 


**Phản ứng dây chuyền (Cascading Failure):** 
   - Tầng Cơ sở dữ liệu: MySQL bị đóng băng tiến trình ghi (Waiting for someone to free space) do cạn kiệt dung lượng phân vùng đĩa (No space left on device), không thể ghi tiếp vào file binlog.   
   - Tầng Ứng dụng (PHP-FPM):  các tiến trình con (worker pools) bị kẹt cứng, dẫn đến cạn kiệt tài nguyên xử lý : 2026/10/20 01:33:16 [error] 2105#0: *5209440 connect() to unix:/run/php/php8.1-fpm.sock failed (11: Resource temporarily unavailable) while connecting to upstream, client: 14.218.58.164, server: site42.vn, request: "GET /tin-tuc/ HTTP/1.1", upstream: "fastcgi://unix:/run/php/php8.1-fpm.sock:", host: "site42.vn"

   - Tầng Gateway (Nginx): Trả về mã lỗi 502/504 do upstream từ PHP-FPM từ chối kết nối (Resource temporarily unavailable) hoặc quá thời gian phản hồi (Gateway Timeout).

## 4. **Các giả thuyết đã loại trừ:**
   Không có. Do các dòng log từ hệ điều hành và MySQL đã chỉ đích danh vấn đề (OS errno 28 - No space left on device), em tập trung rà soát trực tiếp dung lượng ổ cứng để tìm thủ phạm thay vì suy đoán thêm các kịch bản phức tạp khác.

## 5. Hành động khắc phục tức thời
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

## 6. Bài học kinh nghiệm & Hành động phòng ngừa 

Sửa lại lỗi sai chính tả chữ dailly trong file cấu hình /etc/logrotate.d/nginx.

Viết lại đoạn script /usr/local/bin/backup-sites.sh để nó không giữ cả 2 bản backup khổng lồ cùng lúc (như chúng ta vừa phân tích ở trên).

Cấu hình một cái cảnh báo đơn giản (ví dụ script bash check ổ cứng > 75% thì gửi tin nhắn Telegram/Email).