# Nhật ký làm bài

**Họ tên:** Phùng Chí Tài
**Nhận đề lúc:** 29/10/2026
**Nộp bài lúc:** 01/11/2026
**Tổng thời gian làm thật (ước tính):** ~14 giờ

## Các phiên làm việc

| Thời điểm | Thời lượng | Làm phần nào | Ghi chú (kẹt ở đâu, đã thử gì) |
|---|---|---|---|
| Ngày 1 | ~3 giờ | Đánh giá đề & Phần 1 (Postmortem) | Phân tích file JSON để nhận diện các "bẫy" dữ liệu. Dùng AI hỗ trợ lên khung và hoàn thiện báo cáo Postmortem. |
| Ngày 2 | ~5 giờ | Code Go CLI Tool (Phần 2) | Kẹt ở việc thiết kế thuật toán dời VM sao cho tối ưu. Đã quyết định điều phối LLM sinh toàn bộ mã nguồn bằng thuật toán Greedy Best-Fit. |
| Ngày 3 | ~6 giờ | Unit Test (P2), Nghiên cứu (P3) & Báo cáo | Yêu cầu AI làm giàu bộ Unit Test (Table-Driven) để đẩy coverage lên 88.0% và soạn báo cáo so sánh MinIO/Ceph. Xử lý thành công lỗi terminal không nhận lệnh git và xung đột nhánh bằng force push. |
| Ngày 4 | 4 giờ | Tìm kiếm thông tin và phân tích lại nguyên nhân gốc rễ & timeline| Phần 1 | đánh giá sai nguyên nhân gốc rễ là việc nhập sai option "daily" trong service logrotate, thông qua tham chiếu thông tin nhiều chiều đã xác định chính xác nguyên nhân gây ra lỗi 502/504. 
## Giả định đã đặt ra

- Đề không nói rõ VM `stopped` có tính tải hay không: Đã giả định chỉ tính các VM trạng thái `running`.
- Đề không đưa ra mốc quá tải: Đã tự thiết lập ngưỡng an toàn (CPU/RAM > 80%, Overcommit > 2.5x).
- Lỗi dữ liệu `mem_used = null`: Giả định VM này đang chạy full tải và fallback dùng giá trị `mem_max`.

## Câu hỏi đã gửi cho người ra đề

- (Không có - Tự đánh giá các edge cases trong file JSON là bẫy và dùng code fail-safe để tự vượt qua).

## Dùng AI và công cụ

| Bước | Công cụ | Dùng để làm gì | Bạn đã kiểm tra, sửa lại gì |
|---|---|---|---|
| Lập trình Go & Unit Test | LLM (Gemini) | Phân tích cấu trúc file JSON, prompt sinh toàn bộ mã nguồn CLI tool, thuật toán Greedy Best-Fit và bộ Unit Test. | Trực tiếp review lại luồng logic AI đề xuất, cung cấp các giả định ngưỡng tải, tự tay build binary và chạy `go test -v -cover` để nghiệm thu độ chính xác của 8 kịch bản (đạt coverage 88.0%). |
| Nghiên cứu P1, P3 | LLM (Gemini) | Lên dàn ý Postmortem và soạn toàn bộ báo cáo so sánh kiến trúc MinIO vs Ceph RGW. | Đọc chéo lại để kiểm tra tính hợp lý của các luận điểm kỹ thuật, review lại các URL tài liệu chính thức (Official Docs) và tinh chỉnh cấu trúc Markdown. |
| Git & Vận hành | LLM (Gemini) | Hỏi cách xử lý lỗi đường dẫn file JSON khi chạy CLI, lỗi terminal không nhận lệnh và lỗi conflict khi push code. | Chạy thử nghiệm trực tiếp các lệnh trên PowerShell và xác nhận đẩy code lên GitHub thành công. |
|P1| LLM (Gemini)| hỏi logic của đoạn code bash if...then | Không bê nguyên lời giải thích của AI, mà tự ráp logic đó vào bối cảnh thực tế để phát hiện ra cơ chế "nhân đôi dung lượng ổ đĩa" (16GB mới + 19GB cũ) gây tràn đĩa. 
|P1| LLM (Gemini)| Giải mã ý nghĩa của các dòng log phức tạp, các mã lỗi hệ điều hành (như OS errno 28, mã lỗi 11), thông số Nginx (rt=60.587, 552 byte) và phân tích luồng sự kiện đa luồng. |Gạt bỏ các mốc thời gian AI tự suy diễn hoặc đoán mò (như 01:45, 02:15). Tự đối chiếu chéo lại với dữ liệu thô để chốt chính xác các mốc thời gian (như 01:33) và bằng chứng có thật.
Bài học kinh nghiệm cá nhân
|P1| LLM (Gemini)|Tìm kiếm các câu lệnh giải phóng ổ cứng và yêu cầu AI giải thích cơ chế hoạt động.|Thú thực do chưa có kinh nghiệm vận hành thực tế, ban đầu em hoàn toàn phụ thuộc vào gợi ý của AI. Tuy nhiên, em không copy mù quáng mà yêu cầu AI giải thích lệnh truncate và khái niệm inode. Sau đó, em dùng logic để đối chiếu với luật chơi của đề bài: Lệnh này chỉ can thiệp vào /var/log (của hệ thống), tuyệt đối không chạm vào /home hay /var/lib/mysql (của khách hàng). Vì cách giải quyết này thỏa mãn điều kiện an toàn tuyệt đối của đề bài và có cơ sở kỹ thuật thuyết phục, em mới quyết định sử dụng.
*(Ghi chú: Quan điểm làm bài của em là ưu tiên hiệu suất thực tế. Thay vì tự gõ từng dòng code, em tập trung phân tích bài toán, đặt ra các giả định (ngưỡng tải 80%, bẫy lỗi RAM, vCPU) và điều phối AI (Gemini) sinh toàn bộ mã nguồn/tài liệu. Sau đó, em đóng vai trò người nghiệm thu: review logic, chạy test tự động và kiểm thử kết quả CLI trên máy. Cách làm này giúp hoàn thành bài nhanh chóng trong khi vẫn kiểm soát chặt chẽ chất lượng đầu ra).*

## Phần chưa xong hoặc chưa hài lòng

- Trong lần nộp bài đầu tiên, do chưa có kinh nghiệm viết báo cáo sự cố (Postmortem) và tâm lý chủ quan, em đã phụ thuộc quá nhiều vào AI để xử lý log. Em đã lơ là việc đối chiếu chéo (cross-check) kết quả của AI với dữ liệu thô, dẫn đến việc để lọt các mốc thời gian do AI tự suy diễn (như 01:45, 02:15) và kết luận nguyên nhân hời hợt.
- Qua phản hồi của team, em đã nhận ra một bài học đắt giá cho vị trí hệ thống: AI chỉ là công cụ hỗ trợ đọc cú pháp, còn kỹ sư mới là người phải chịu trách nhiệm rà soát từng dòng log và đưa ra quyết định cuối cùng. Ở bản cập nhật này, em đã tự dò lại toàn bộ file syslog, nginx log để đảm bảo 100% các mốc thời gian và lỗi sai chính tả (dailly) đều là dữ liệu thực tế.
- Giao diện dạng bảng (Terminal UI) ở Phần 2 đang dùng thư viện chuẩn `text/tabwriter`. Nếu có thêm thời gian, em sẽ dùng thư viện `charmbracelet/bubbletea` để tạo giao diện tương tác trực quan hơn.

## Nếu làm lại, bạn sẽ làm khác điều gì?

- Thay vì đọc file JSON tĩnh, em sẽ nâng cấp tool gọi trực tiếp API của Proxmox VE để tool có thể chạy real-time như một cronjob tự động alert/migrate.