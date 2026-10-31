# Nhật ký làm bài

**Họ tên:** Phùng Chí Tài
**Nhận đề lúc:** 29/10/2026
**Nộp bài lúc:** 31/10/2026
**Tổng thời gian làm thật (ước tính):** ~14 giờ

## Các phiên làm việc

| Thời điểm | Thời lượng | Làm phần nào | Ghi chú (kẹt ở đâu, đã thử gì) |
|---|---|---|---|
| Ngày 1 | ~3 giờ | Đánh giá đề & Phần 1 (Postmortem) | Phân tích file JSON để nhận diện các "bẫy" dữ liệu. Hoàn thiện báo cáo Postmortem. |
| Ngày 2 | ~5 giờ | Code Go CLI Tool (Phần 2) | Kẹt ở việc thiết kế thuật toán dời VM sao cho tối ưu. Đã quyết định sử dụng LLM để hỗ trợ sinh mã nguồn bằng thuật toán Greedy Best-Fit. |
| Ngày 3 | ~6 giờ | Unit Test (P2), Nghiên cứu (P3) & Báo cáo | Tốn thời gian làm giàu bộ Unit Test (Table-Driven) để đẩy coverage lên 88.0%. Tra cứu tài liệu chính thức (Official Docs) để chốt so sánh MinIO/Ceph. Xử lý thành công lỗi terminal không nhận lệnh git bằng force push. |

## Giả định đã đặt ra

- Đề không nói rõ VM `stopped` có tính tải hay không: Đã giả định chỉ tính các VM trạng thái `running`.
- Đề không đưa ra mốc quá tải: Đã tự thiết lập ngưỡng an toàn (CPU/RAM > 80%, Overcommit > 2.5x).
- Lỗi dữ liệu `mem_used = null`: Giả định VM này đang chạy full tải và fallback dùng giá trị `mem_max`.

## Câu hỏi đã gửi cho người ra đề

- (Không có - Tự đánh giá các edge cases trong file JSON là bẫy và dùng code fail-safe để tự vượt qua).

## Dùng AI và công cụ

| Bước | Công cụ | Dùng để làm gì | Bạn đã kiểm tra, sửa lại gì |
|---|---|---|---|
| Lập trình Go & Unit Test | LLM (Gemini) | Sinh mã nguồn Go (struct, làm sạch dữ liệu, Best-Fit). Sinh bộ Table-Driven Tests bao phủ 8 kịch bản bẫy dữ liệu. | Đã đọc hiểu luồng xử lý, chạy thực tế lệnh `go test -v -cover` trên máy cá nhân, xác nhận đạt coverage 88.0% và pass toàn bộ kịch bản. |
| Nghiên cứu P1, P3 | LLM (Gemini) | Tham khảo dàn ý Postmortem SRE và tìm kiếm các link tài liệu gốc (Ceph Docs, MinIO Docs, Source code). | Đọc chéo lại tài liệu hãng để đảm bảo tính logic, kiểm chứng các số liệu benchmark công bố chính thức và tinh chỉnh lại văn phong. |


*(Ghi chú: Quan điểm làm bài của em là ưu tiên tính hiệu quả. Em dùng AI để sinh mã nguồn nhanh và lấy dẫn chứng tài liệu, sau đó tập trung vào việc đọc/chạy Unit Test để nghiệm thu. Chỉ cần tool chạy ra đúng output, tránh được hết các bẫy dữ liệu ngoại lệ trong JSON là đạt yêu cầu. Do đây là CLI tool chạy local đọc file tĩnh, em ưu tiên xử lý độ ổn định/chống crash thay vì bận tâm các yếu tố bảo mật dư thừa).*

## Phần chưa xong hoặc chưa hài lòng

- Giao diện dạng bảng (Terminal UI) ở Phần 2 đang dùng thư viện chuẩn `text/tabwriter`. Nếu có thêm thời gian, em sẽ dùng thư viện `charmbracelet/bubbletea` để tạo giao diện tương tác trực quan hơn.

## Nếu làm lại, bạn sẽ làm khác điều gì?

- Thay vì đọc file JSON tĩnh, em sẽ nâng cấp tool gọi trực tiếp API của Proxmox VE để tool có thể chạy real-time như một cronjob tự động alert/migrate.