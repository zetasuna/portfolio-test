# Báo cáo kiểm chứng

Tài liệu này phân biệt rõ những kiểm tra đã thực thi trong môi trường tạo gói với những kiểm tra cần Docker, PostgreSQL hoặc truy cập mạng. Không đánh dấu PASS cho bước chưa chạy.

## Giới hạn môi trường đóng gói

- Toolchain có sẵn là Go 1.23.2, trong khi dự án cố định `go 1.27.0` và `toolchain go1.27.2`.
- Khi dùng toolchain có sẵn, `GOTOOLCHAIN=local go test ./...` dừng trước biên dịch vì phiên bản Go không đáp ứng yêu cầu module. Khi để Go tự tải toolchain, lần tải Go 1.27.2 thất bại vì DNS/kết nối tới `proxy.golang.org` bị từ chối.
- Không có Docker CLI hoặc Docker Compose trong môi trường đóng gói; vì vậy không thể kiểm tra `docker compose config` bằng Compose thực, build image hoặc khởi động 12 services.
- Dependency cache không đầy đủ và không tải được các Go modules; không thể sinh `go.sum` tin cậy ở đây. ZIP không chứa `go.sum`. Trên máy có Go 1.27.2 và Internet, chạy `go mod tidy`, rà soát và commit `go.sum` trước khi coi release là tái lập được.

## Kiểm tra đã thực thi và đạt

- `gofmt -w cmd internal tests` và xác nhận `gofmt -l cmd internal tests` không trả file nào.
- `GO111MODULE=off go test ./internal/storage` — PASS; đây là package test độc lập dùng thư viện chuẩn Go.
- `bash -n` cho tất cả `scripts/*.sh` và `sh -n deploy/postgres/init/01-app-role.sh` — PASS.
- Đọc cú pháp bằng PyYAML cho 8 file YAML/YML — PASS.
- Parse JSON của dashboard Grafana — PASS.
- Kiểm tra hợp đồng Compose bằng PyYAML: đủ đúng 12 service bắt buộc; app1/app2/app3 không publish port; PostgreSQL, Prometheus, Loki, Tempo và OTLP Collector không publish cổng ra host — PASS ở mức cấu trúc YAML, **không phải** `docker compose config`.
- Parse cả 9 Go HTML templates bằng `html/template` và helper Go tạm — PASS.

## Kiểm tra chưa thể chạy

Các bước dưới đây chưa được xác minh runtime trong môi trường đóng gói:

- Toàn bộ `go test ./...`, `go vet ./...`, race detector và coverage — bị chặn bởi phiên bản Go cũ và không tải được dependencies.
- Docker Compose interpolation/validation, build image, container healthchecks, database migrations trên PostgreSQL thật.
- Browser CRUD/auth/upload, load balancing/failover Nginx và restart persistence.
- GitHub webhook, Jenkins pipeline trên daemon thật, Docker Hub push, triển khai và rollback.
- Metrics thực tế từ cả ba app instances, datasource health trên Grafana, log ingestion/query từ Loki, trace HTTP + PostgreSQL xuất hiện trong Tempo.
- Dependency vulnerability scan và container image vulnerability scan.

## Các lệnh kiểm chứng trên host có Docker và Go 1.27.2

```bash
# Tạo checksum module từ mạng đáng tin cậy và kiểm tra source
go mod tidy
gofmt -w cmd internal tests
go vet ./...
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -n 1

# Khởi động stack
./scripts/setup.sh
docker compose config
docker compose up -d --build
docker compose ps
docker compose logs --tail=200

# CRUD, load balancing, integration và observability
./scripts/create-admin.sh
./scripts/smoke.sh
./scripts/curl-crud.sh
./scripts/integration-test.sh
./scripts/verify-observability.sh
./scripts/acceptance.sh
```

Kiểm tra thêm trong Prometheus rằng targets `app1`, `app2`, `app3` đều có `up == 1`; Grafana có datasources Prometheus/Loki/Tempo; Loki trả về log `portfolio-app`; Tempo có trace gần đây chứa HTTP span và span thao tác PostgreSQL. Xác minh `trace_id` trên log cùng request khớp trace trong Tempo.

Đây là checklist để chạy sau khi giải nén, không phải kết quả test đã chạy sẵn. Xem `README.md` để biết cách cấu hình và xử lý lỗi.
