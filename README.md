# Waaza

Waaza adalah REST API WhatsApp berbasis `whatsmeow` dengan dashboard web, role-based access, dan mode async queue untuk pengiriman pesan.

> Fokus: maintainable, aman untuk production, dan kompatibel pola endpoint ala WUZAPI.

## Fitur Utama

- Multi-instance management (admin)
- Role access:
  - **Admin**: akses semua instance
  - **User**: hanya instance miliknya (berdasarkan token instance)
- Session control:
  - connect / disconnect / logout / status
  - QR pairing untuk scan WhatsApp Linked Devices
- Chat tools:
  - send text (sudah async queue)
  - scaffolding untuk media/group tools
- Webhook & config panel di dashboard
- PostgreSQL persistence untuk users, instances, dan outbox queue
- Systemd-ready deployment

## Arsitektur Ringkas

- `cmd/server` → entrypoint
- `internal/api` → handler + routing
- `internal/service` → business logic + outbox worker
- `internal/wa` → adapter ke `whatsmeow`
- `internal/store` → persistence layer (Postgres)
- `web/templates` → dashboard + UI
- `openapi/spec.yml` → spesifikasi API

## Requirements

- Go >= 1.25
- PostgreSQL >= 14 (tested with 16)
- Linux server (tested on Ubuntu)

## Environment Variables

| Variable | Keterangan | Contoh |
|---|---|---|
| `PORT` | Port HTTP server | `8090` |
| `WAAZA_PROVIDER` | Provider WA | `whatsmeow` |
| `WAAZA_DB_DRIVER` | DB driver sqlstore | `pgx` |
| `WAAZA_DB_DSN` | DSN PostgreSQL | `postgres://waaza:pass@127.0.0.1:5432/waaza?sslmode=disable` |
| `WAAZA_API_KEY` | User API key global | `change-me-user-token` |
| `WAAZA_ADMIN_TOKEN` | Admin token | `change-me-admin-token` |

## Menjalankan Lokal

```bash
cd waaza
export PORT=8090
export WAAZA_PROVIDER=whatsmeow
export WAAZA_DB_DRIVER=pgx
export WAAZA_DB_DSN='postgres://waaza:password@127.0.0.1:5432/waaza?sslmode=disable'
export WAAZA_API_KEY='change-me-user-token'
export WAAZA_ADMIN_TOKEN='change-me-admin-token'

go run ./cmd/server
```

Akses:
- Dashboard: `http://localhost:8090/dashboard`
- Swagger UI: `http://localhost:8090/api/`
- OpenAPI: `http://localhost:8090/api/spec.yml`
- Health: `http://localhost:8090/health`

## Async Queue (Fast Mode)

Send text endpoint saat ini memakai outbox queue (durable di Postgres):

- enqueue cepat (`status: queued`, `queue_id`)
- worker pickup cepat (~120ms tick)
- retry otomatis (exponential backoff)
- status queue bisa dicek via endpoint

Status outbox:
- `pending`
- `processing`
- `sent`
- `failed`
- `dead`

## Endpoint Penting (current)

### Public
- `GET /health`
- `POST /auth/verify`
- `GET /api/`
- `GET /api/spec.yml`

### User scope
- `GET /instances/me`
- `GET /instances/me/:id`
- `POST /instances/me/:id/session/connect`
- `GET /instances/me/:id/session/qr`
- `POST /instances/me/:id/chat/send/text`

### Admin scope
- `GET /admin/instances`
- `POST /admin/instances`
- `GET /admin/instances/:id`
- `DELETE /admin/instances/:id`
- `POST /admin/instances/:id/session/connect`
- `POST /admin/instances/:id/chat/send/text`

### Queue
- `GET /queue/:id`

## Deployment via systemd

Contoh unit & env ada di folder `deploy/`.

File penting:
- `deploy/waaza.service`
- `deploy/waaza.env.example`

Setelah env siap:

```bash
sudo cp deploy/waaza.service /etc/systemd/system/waaza.service
sudo mkdir -p /etc/waaza
sudo cp deploy/waaza.env.example /etc/waaza/waaza.env
sudo chmod 600 /etc/waaza/waaza.env

sudo systemctl daemon-reload
sudo systemctl enable --now waaza
sudo systemctl status waaza
```

## Keamanan (Wajib)

- **Jangan commit** token/API key/password ke repo
- Simpan kredensial di `/etc/waaza/waaza.env` (permission `600`)
- Rotasi:
  - `WAAZA_ADMIN_TOKEN`
  - `WAAZA_API_KEY`
  - password PostgreSQL
- Review log sebelum expose publik
- Batasi akses jaringan (mis. hanya Tailscale/VPN)

## .gitignore policy

Pastikan file berikut tidak pernah masuk Git:

- `.env`
- `*.env`
- `waaza.db`
- `*.db`
- `deploy/.postgres-created.txt`
- file secret/backup kredensial

## Roadmap singkat

- [ ] Real contacts/groups actions via whatsmeow
- [ ] Queue list/retry/cancel endpoint
- [ ] Token hashing & auth hardening
- [ ] Trusted proxies + security headers
- [ ] CI test + release pipeline

---

Jika dipakai production: lakukan hardening dulu sebelum internet-facing.
