# DNS Management Backend

Go backend for DNS Hub with MySQL persistence, JWT login, admin approval, API tokens, and DNS lookup through Cloudflare `1.1.1.1` DNS-over-HTTPS.

## Run

Create a MySQL database, then run:

```powershell
cd backend
$env:MYSQL_DSN='root:password@tcp(127.0.0.1:3306)/dns_management?charset=utf8mb4&parseTime=true&loc=Local'
$env:JWT_SECRET='change-me'
$env:TOKEN_ENCRYPTION_KEY='change-me-too'
go run .
```

The service listens on `:8080` by default and creates tables automatically. It does not create any default users, domains, subdomains, or DNS records.

## First Setup

When the `users` table is empty, the frontend login page switches to first-run setup. The first submitted account is created as an administrator.

Public setup endpoints:

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/setup/status` | Returns `{ "initialized": false }` when no user exists |
| `POST` | `/api/v1/setup/admin` | Creates the first admin; only works while no user exists |

After setup, sign in with that administrator and add one or more root domains in `/admin`.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `MYSQL_DSN` | required | MySQL DSN for `github.com/go-sql-driver/mysql` |
| `JWT_SECRET` | `dev-change-me` | JWT signing secret |
| `TOKEN_ENCRYPTION_KEY` | `JWT_SECRET` | Secret used to encrypt Cloudflare API tokens |
| `ADDR` | `:8080` | HTTP listen address |

## API

All endpoints are under `/api/v1`. Use `Authorization: Bearer <jwt>` after login.

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/login` | Email/password login |
| `POST` | `/auth/change-password` | Current user password change |
| `GET` | `/me` | Current user |
| `GET` | `/domains/enabled` | Enabled root domains |
| `GET` | `/subdomains` | Current user's subdomains |
| `POST` | `/subdomains` | Apply for a subdomain with `{ "domainId": "...", "prefix": "blog" }` |
| `GET` | `/subdomains/{id}/records` | List DNS records |
| `POST` | `/subdomains/{id}/records` | Create local DNS record |
| `DELETE` | `/subdomains/{id}/records/{recordId}` | Delete local DNS record |
| `GET` | `/tokens` | List API token metadata |
| `POST` | `/tokens` | Create API token; plaintext token is returned once |
| `DELETE` | `/tokens/{id}` | Revoke API token |
| `GET` | `/dns-query?name=example.com&type=A` | Query DNS through Cloudflare DoH |
| `GET` | `/admin/domains` | Admin domain list |
| `POST` | `/admin/domains` | Admin add root domain and Cloudflare token |
| `PATCH` | `/admin/domains/{id}` | Admin update domain |
| `DELETE` | `/admin/domains/{id}` | Admin delete domain |
| `GET` | `/admin/subdomains?status=pending` | Admin approval queue |
| `POST` | `/admin/subdomains/{id}/approve` | Approve request |
| `POST` | `/admin/subdomains/{id}/reject` | Reject request |
| `GET` | `/admin/users` | Admin user list |
| `POST` | `/admin/users/{id}/reset-password` | Admin reset user password |
