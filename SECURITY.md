# Security notes

## Scope

This project is a learning/homelab application. It includes password hashing (Argon2id), opaque server-side sessions, CSRF validation for state-changing admin actions, rate limiting, host validation, security response headers, HTML templates with contextual escaping, parameterized database operations, HTTPS-only external URLs, bounded image uploads and non-root application containers.

These controls reduce common risks but do not guarantee the system is secure. Source review, dependency/image scans, runtime tests, backups, host hardening and incident response remain necessary.

## Assumptions

- The default deployment is local-only: the Nginx website, Grafana and Jenkins host ports bind to `127.0.0.1`.
- Docker Engine and the host are trusted administrative boundaries.
- `.env` and Jenkins Credentials are readable only by authorized operators.
- If a TLS reverse proxy is added, it is trusted, overwrites forwarded headers, and is the only path to the application.
- Docker uses the `json-file` logging driver for Alloy's configured log path.
- The Go application connects as a non-superuser PostgreSQL role; a separate bootstrap role initializes the data directory.

## High-impact residual risks

### Jenkins Docker socket

The Jenkins container mounts `/var/run/docker.sock`. Access to the Docker daemon commonly permits operations equivalent to root on the Docker host, including starting privileged containers or mounting host filesystems. Jenkins is intentionally not configured with Docker `privileged: true`, but socket access remains powerful. Bind its web port to localhost/VPN, restrict Jenkins administrators, protect credentials, and do not execute Jenkinsfiles from forks/untrusted pull requests. For higher-assurance use, move builds to isolated agents or a remote builder with restricted permissions.

### Single points of failure and storage

Three app containers do not provide whole-system high availability. Nginx, PostgreSQL, host storage, Jenkins, and observability backends can each be a single point of failure. Named volumes do not replace off-host backups. Uploads use shared local Docker storage and are not suitable for independent multi-host deployment without redesign.

### Internet exposure

Do not expose PostgreSQL, Prometheus, Loki, Tempo, OTLP, or Docker API ports. Keep Jenkins and Grafana on loopback or a protected management network. Before exposing the website, provide HTTPS on a trusted edge proxy, configure `COOKIE_SECURE=true`, set the correct `PUBLIC_BASE_URL` and `ALLOWED_HOSTS`, apply firewall rules and validate redirect/header behavior.

### Trusted proxy headers

`TRUST_PROXY_HEADERS=true` is appropriate only when every request to the application passes through a proxy that overwrites `X-Real-IP`. If a client can reach the app directly, spoofed headers can undermine IP-based rate limiting. Do not publish app ports to the host.

### Uploads

The application accepts JPEG and PNG, enforces a 5 MiB application limit and checks image structure/dimensions. SVG and WebP are not accepted. Image decoders are still attack surface, so keep Go and dependencies updated, retain request limits and consider malware scanning if the application is used beyond a private lab.

### Sessions, passwords and CSRF

Admin passwords are hashed with Argon2id. Cookie flags depend on deployment mode; `Secure` must be enabled for HTTPS. CSRF tokens are required for state-changing forms. Session tokens are random and their hashes, rather than raw tokens, are stored in PostgreSQL. Keep system clocks correct and rotate admin credentials if compromise is suspected.

## Database privilege boundary

The PostgreSQL image's `POSTGRES_USER` is a bootstrap administrator, not the application account. Initialization creates `APP_DB_USER` with `NOSUPERUSER`, `NOCREATEDB`, and `NOCREATEROLE`; the application gets only connection access and schema `USAGE`/`CREATE` required for versioned migrations. A separate `TEST_DB_NAME` database is provisioned for integration tests. The init script runs only when the PostgreSQL data directory is first created; changing `.env` later does not rotate existing database credentials automatically.

## Secret management

- Never commit `.env`, real passwords, Docker Hub tokens, GitHub tokens or Jenkins secret files.
- Use `./scripts/setup.sh` for random local secrets and restrictive `.env` permissions.
- Use Jenkins Credentials IDs documented in README; do not interpolate secrets into command arguments where avoidable.
- Use a dedicated, least-privilege Docker Hub token.
- Rotate exposed credentials and rebuild/redeploy after a secret leak; deleting a Git commit is not sufficient if it has been pushed.

## HTTPS deployment checklist

1. Terminate TLS using a maintained reverse proxy with valid certificate and automatic renewal.
2. Permit public inbound traffic only to ports 80/443 at the edge; keep admin/observability ports on localhost/VPN.
3. Set `COOKIE_SECURE=true`, `PUBLIC_BASE_URL=https://your-domain` and include the hostname in `ALLOWED_HOSTS`.
4. Ensure the proxy overwrites `X-Real-IP`, `X-Forwarded-For` and `X-Forwarded-Proto`; configure trust only for known proxies.
5. Review HSTS only after HTTPS is fully verified and all relevant subdomains are ready.
6. Verify login/logout, CSRF rejection, upload limits, redirects, logs and tracing through the TLS entry point.
7. Back up PostgreSQL and uploads, and test restore.

## Security validation

Run `go test ./...`, `go vet ./...`, `govulncheck ./...`, an image scan such as Trivy and the scripts in `scripts/` in an environment with dependencies installed and Docker running. Review every HIGH/CRITICAL scanner finding; do not assume scans establish absence of vulnerabilities. See `VERIFICATION.md` for the checks run during packaging.
