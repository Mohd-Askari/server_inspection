# CoreIP Server Inspector

[![Go Version](https://img.shields.io/badge/go-1.24%2B-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-Ubuntu%20%7C%20CentOS%20Stream%20%7C%20RHEL-orange.svg)](https://kernel.org)
[![Architecture](https://img.shields.io/badge/arch-amd64-green.svg)](https://en.wikipedia.org/wiki/X86-64)
[![License](https://img.shields.io/badge/license-Proprietary-red.svg)](#license)

**CoreIP Server Inspector** is a production-grade, lightweight Linux server hardware inventory and health monitoring application designed for IT operations and support engineering teams.

It provides a single-pane-of-glass web dashboard and REST API for inspecting Ubuntu and CentOS/RHEL-compatible servers, eliminating the need to manually SSH and execute multiple ad-hoc CLI commands (`lshw`, `dmidecode`, `lsblk`, `smartctl`, `systemctl`, `ip`, `lspci`).

---

## Key Highlights

- **Zero Heavy Runtime Dependencies**: Implemented in Go. No external database, Docker, Kubernetes, or Node.js runtime required.
- **Embedded Web UI**: Self-contained single binary embedding HTML5, responsive CSS, and vanilla JS with zero external frontend libraries or CDNs.
- **True Linux Memory Semantics**: Accurately computes memory pressure using `MemAvailable` and slab reclaimable caches rather than simplistic free memory formulas.
- **Storage Diagnostics & Physical vs Mounted Separation**: Clear physical block device inventory (NVMe SSD, SATA SSD, HDD, Virtual), partition mount table, usage warnings (>80% warning, >90% critical), and non-destructive read-only SMART diagnostics via optional `smartctl`.
- **Accurate CPU Topology**: Strictly differentiates physical sockets, physical cores, and logical hyperthreaded processors/vCPUs.
- **Secure by Default**:
  - Binds strictly to `127.0.0.1` unless remote access is explicitly configured.
  - Remote listening mandates authentication.
  - Strong password hashing with bcrypt, cryptographically random session tokens, `SameSite=Strict` HTTP-only cookies, CSRF protection, and brute-force rate limiting.
  - Security audit log for logins, logouts, report exports, and config inspections.
  - Designed to run unprivileged under a dedicated `coreip` system user.
- **Production Packaging**: Ready-to-deploy `.deb` (Ubuntu 22.04/24.04, Debian) and `.rpm` (CentOS Stream 9, RHEL 9.x) packages with hardened systemd service unit.

---

## Architecture Overview

```
coreip-server-inspector/
├── cmd/
│   ├── inspector/main.go            # Primary application entry point & CLI flags
│   └── package-deb/main.go          # Portable Debian package builder (.deb)
├── configs/
│   ├── config.yaml                  # Production configuration file with safe defaults
│   └── coreip-inspector.service     # Hardened systemd service unit
├── internal/
│   ├── api/                         # REST API handlers, routing & security middleware
│   ├── auth/                        # Bcrypt auth, sessions, CSRF, rate limiter & audit log
│   ├── collector/                   # Subsystem collectors (/proc, /sys, tools)
│   ├── config/                      # YAML configuration loader & validator
│   ├── metrics/                     # In-memory historical ring buffer
│   ├── models/                      # Domain data models & JSON schemas
│   └── reports/                     # Plaintext TXT and JSON report generators
├── packaging/
│   ├── debian/                      # Debian control, postinst, prerm, build-deb.sh
│   └── rpm/                         # RPM .spec file, build-rpm.sh
└── web/                             # Embedded single-page application (HTML/CSS/JS)
```

---

## Navigation & Web Interface Sections

The CoreIP Server Inspector web interface provides 10 dedicated sections:

1. **Dashboard**: Summary cards (CPU cores, RAM total/available, physical storage, current utilization, failed services, SMART summary), live 10-minute historical graphs for CPU, RAM, and Network traffic, and active alerts.
2. **System Identity**: OS name, distribution, version, kernel version, architecture, uptime, boot time, manufacturer, model, serial number, BIOS version/date, and virtualization/hypervisor detection (KVM, VMware, Hyper-V, VirtualBox, containers).
3. **CPU & Memory**: Processor topology (physical sockets, physical cores, logical processors, vCPUs), frequencies, cache topology, per-core utilization bars, load averages, and Linux memory breakdown (Total, Used, Available, Free, Buffers, Cached, Swap).
4. **Storage & SMART**:
   - Physical block drives: name, path, NVMe/SATA/HDD classification, rotational status, capacity, serial number, and read-only flags.
   - Mounted filesystems: mount points, filesystem types, capacity, utilization, and read-only warnings.
   - Software RAID & LVM device-mapper layers.
   - SMART disk health: overall health status (Healthy, Warning, Critical, Unsupported, Unavailable), temperatures, power-on hours, and reallocated bad sectors.
5. **GPU & Accelerators**: PCI display adapters, NVIDIA telemetry (via `nvidia-smi` when present), VRAM total/used, utilization, temperatures, and power draw. Gracefully indicates when no GPU is installed.
6. **Network Interfaces**: Network adapters, state (UP/DOWN), MAC addresses, link speeds, duplex, MTU, assigned IPv4 and IPv6 addresses, error/drop counters, live RX/TX rates, default gateway, and DNS servers.
7. **Services**: systemd units overview, active vs failed count, real-time search filtering, state filtering (all, active, failed, inactive), and pagination.
8. **Health & Alerts**: Aggregated anomaly detection (storage >80% warning / >90% critical, root mounted read-only, memory pressure >85%, failed services, SMART errors) and subsystem collector statuses.
9. **System Reports**: In-browser report preview and one-click downloads in structured JSON or formatted ASCII plaintext (`.txt`).
10. **Settings & Access Configuration**: Security posture review, bind address, authentication state, polling frequency, and production deployment recommendations.

---

## Quick Start & Local Execution

### Build from Source
```bash
# Clone the repository
cd coreip-server-inspector

# Compile native binary
make build

# Run unit tests
make test

# Start the inspector locally
./bin/coreip-inspector -config configs/config.yaml
```

The inspector will bind to `http://127.0.0.1:8080`. Open your browser to view the dashboard.

### Default Operator Credentials
- **Username**: `admin`
- **Initial Password**: `CoreIPAdmin2026!` *(Configured in `configs/config.yaml`)*

Generate a custom bcrypt password hash prior to deployment:
```bash
./bin/coreip-inspector -generate-password-hash "YourStrongPasswordHere"
```

---

## Installation & Packaging

### Ubuntu & Debian (.deb)

Build or obtain the `.deb` package:
```bash
make package-deb
# Generated at: dist/coreip-server-inspector_1.0.0_amd64.deb
```

Install on target server:
```bash
sudo apt install ./dist/coreip-server-inspector_1.0.0_amd64.deb

# Enable and start systemd service
sudo systemctl enable --now coreip-inspector

# Check service status
sudo systemctl status coreip-inspector
```

### CentOS Stream & RHEL 9 (.rpm)

Build or obtain the `.rpm` package:
```bash
make package-rpm
# Spec file: packaging/rpm/coreip-inspector.spec
```

Install on target server:
```bash
sudo dnf install ./dist/coreip-server-inspector-1.0.0-1.x86_64.rpm

# Enable and start systemd service
sudo systemctl enable --now coreip-inspector

# Check service status
sudo systemctl status coreip-inspector
```

---

## Configuration Reference (`config.yaml`)

```yaml
server:
  host: "127.0.0.1"          # Bind address. Default: localhost loopback only
  port: 8080                 # Listening port
  allow_remote: false        # Must be explicitly true to bind to 0.0.0.0 or LAN IP
  tls_cert_file: ""          # Optional direct HTTPS certificate path
  tls_key_file: ""           # Optional direct HTTPS private key path

auth:
  enabled: true              # Mandatory when allow_remote is true
  username: "admin"
  password_hash: ""          # Bcrypt hash (e.g. $2a$12$...)
  initial_password: "CoreIPAdmin2026!" # Used if password_hash is empty
  session_timeout_minutes: 60
  rate_limit_per_minute: 10  # Brute-force lockout threshold

collector:
  poll_interval_seconds: 5   # Real-time dashboard update rate
  cache_inventory_seconds: 30 # Hardware inventory cache duration
  history_buffer_size: 120   # Retain 10 minutes of samples at 5s intervals
  service_max_items: 200     # Upper bound on enumerated systemd units
  smart_enabled: true        # Enable disk SMART inspection
  smart_cache_seconds: 300   # Run read-only smartctl once per 5 minutes

logging:
  level: "info"
  audit_log_path: ""         # Path to audit log file (empty = stdout)
```

---

## Security Architecture & Remote Access

### Security Guarantees
1. **Loopback-Only by Default**: The application refuses to start on non-loopback addresses unless `server.allow_remote: true` is configured.
2. **Mandatory Remote Authentication**: Remote mode strictly enforces valid session tokens.
3. **Session Cookies**: Cryptographically secure 256-bit session tokens stored with `HttpOnly`, `SameSite=Strict`, and `Secure` (when TLS enabled).
4. **CSRF Defense**: State-modifying endpoints (`/api/v1/auth/logout`) require valid `X-CSRF-Token` headers matching the authenticated session.
5. **Rate Limiting**: Sliding-window IP rate limiter protects against brute-force login attempts (HTTP 429).
6. **Strict Security Headers**: Automatic `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, and `Content-Security-Policy`.
7. **Read-Only Diagnostics**: Never initiates destructive disk formatting, self-tests, or service restarts. No shell execution (`exec.Command` with argument slices, never `sh -c`).
8. **Audit Trail**: Security-relevant events (logins, failures, logouts, report exports) are logged with client IP without sensitive credentials.

### Recommended Remote Deployment Patterns

#### Pattern A: Internal HTTPS Reverse Proxy (Recommended)
Deploy CoreIP Inspector bound to `127.0.0.1:8080`, and terminate TLS with Nginx on the server:

```nginx
server {
    listen 443 ssl http2;
    server_name inspector.internal.company.com;

    ssl_certificate     /etc/ssl/certs/internal.crt;
    ssl_certificate_key /etc/ssl/private/internal.key;

    # Restrict to support subnet / VPN
    allow 10.0.0.0/8;
    allow 192.168.1.0/24;
    deny all;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

#### Pattern B: Direct HTTPS with Built-in TLS
Configure certificates directly in `config.yaml`:
```yaml
server:
  host: "0.0.0.0"
  port: 8443
  allow_remote: true
  tls_cert_file: "/etc/coreip-inspector/tls.crt"
  tls_key_file: "/etc/coreip-inspector/tls.key"
```

---

## REST API Reference

| Method | Endpoint | Description | Auth Required |
|---|---|---|---|
| `GET` | `/api/v1/health` | Overall health summary, active issues, collector states | No |
| `POST` | `/api/v1/auth/login` | Authenticate with credentials and receive session cookie | No (Rate Limited) |
| `GET` | `/api/v1/auth/session` | Inspect active session status and CSRF token | No |
| `POST` | `/api/v1/auth/logout` | Revoke session and clear cookie | Yes (Requires CSRF) |
| `GET` | `/api/v1/system` | Operating system, kernel, DMI, and virtualization identity | Yes |
| `GET` | `/api/v1/hardware` | Complete hardware inventory across all subsystems | Yes |
| `GET` | `/api/v1/metrics/current` | Real-time CPU, RAM, swap, and traffic rate sample | Yes |
| `GET` | `/api/v1/metrics/history` | Historical metric series from the bounded ring buffer | Yes |
| `GET` | `/api/v1/storage` | Physical block drives and mounted partitions | Yes |
| `GET` | `/api/v1/storage/health` | SMART drive diagnostics | Yes |
| `GET` | `/api/v1/services` | systemd service units with `filter`, `page`, `page_size` | Yes |
| `GET` | `/api/v1/network` | Network adapters, IP addresses, counters, and DNS | Yes |
| `GET` | `/api/v1/report` | Export diagnostic report (`format=json` or `format=txt`) | Yes |
| `GET` | `/api/v1/config` | Redacted server configuration and access mode | Yes |

---

## Diagnostic Utility Detection & Permissions

CoreIP Server Inspector operates without root privileges. Optional utilities are detected automatically:

| Utility | Purpose | Package (Ubuntu/Debian) | Package (RHEL/CentOS) | Permission Requirement |
|---|---|---|---|---|
| `smartctl` | Physical drive SMART health | `smartmontools` | `smartmontools` | Unprivileged reads / `CAP_SYS_RAWIO` |
| `systemctl` | systemd unit monitoring | Built-in | Built-in | Unprivileged user access |
| `nvidia-smi`| GPU telemetry | `nvidia-utils-*` | `xorg-x11-drv-nvidia-cuda` | Vendor driver installed |

If an optional utility is missing, the inspector gracefully marks the subsystem as `unavailable` or `unsupported` and continues operating normally without errors.

---

## Uninstallation

### Ubuntu/Debian
```bash
sudo systemctl disable --now coreip-inspector
sudo apt remove --purge coreip-server-inspector
sudo rm -rf /etc/coreip-inspector /var/log/coreip-inspector
```

### CentOS/RHEL
```bash
sudo systemctl disable --now coreip-inspector
sudo dnf remove coreip-server-inspector
sudo rm -rf /etc/coreip-inspector /var/log/coreip-inspector
```

---

## Testing & Compatibility

- **Target Platforms**: Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, CentOS Stream 9, RHEL 9.x.
- **Cross-Platform Development**: Includes full support for non-Linux hosts (macOS) with mock fixtures and testable abstractions.
- **Test Suite**: Run `make test` or `make test-race` to verify unit tests, fixture parsing, and concurrency guarantees.

