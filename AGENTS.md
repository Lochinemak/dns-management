# Repository Guidelines

## Project Structure & Module Organization

This repository contains a DNS management service with a Go API and a Next.js frontend. The backend lives in `backend/`; `main.go` contains the HTTP server, migrations, auth, DNS record logic, and static frontend serving, while `main_test.go` holds unit tests. The frontend lives in `frontend/`; routes are under `app/`, DNS feature components under `components/dns/`, shared UI primitives under `components/ui/`, and API/helpers under `lib/`. Root deployment examples include `Dockerfile`, `docker-compose-sample.yml`, and `nginx-sample.conf`.

## Build, Test, and Development Commands

- `cd backend && go run .`: run the API on `ADDR` or `:8080`; requires `MYSQL_DSN`.
- `cd backend && go test ./...`: run all Go tests.
- `cd frontend && npm install`: install dependencies from `package-lock.json`.
- `cd frontend && npm run dev`: start the Next.js dev server.
- `cd frontend && npm run build`: build the frontend for production.
- `cd frontend && npm run lint`: run ESLint across the frontend.
- `docker compose -f docker-compose-sample.yml up --build`: run MySQL and the bundled app.

## Coding Style & Naming Conventions

Go code must be formatted with `gofmt`; keep tests in `*_test.go` files with `TestXxx` names. Use idiomatic Go identifiers: exported types use `PascalCase`, unexported helpers use `camelCase`.

Frontend code uses TypeScript `strict` mode, Next.js App Router, and the `@/*` path alias. Use two-space indentation in `.ts` and `.tsx` files, `PascalCase` for React components, and `camelCase` for functions, props, and state. Prefer existing `components/ui/` primitives and `lucide-react` icons.

## Testing Guidelines

Backend tests use Go's `testing` package plus `go-sqlmock` for database expectations. Add focused tests for authentication, authorization, validation, migrations, and route behavior when changing API logic. Frontend tests are not configured, so verify frontend changes with `npm run lint` and `npm run build`.

## Commit & Pull Request Guidelines

Recent commits use short, imperative summaries such as `Serve exported frontend clean URLs` and `Fix frontend lockfile for Docker build`; one commit is Chinese (`增加dns record edit`). Keep commits concise and scoped. Pull requests should include a brief description, verification commands, linked issues when applicable, and screenshots for UI changes.

## Security & Configuration Tips

Do not commit real secrets. Configure the backend with `MYSQL_DSN`, `JWT_SECRET`, `TOKEN_ENCRYPTION_KEY`, and optionally `ADDR` and `FRONTEND_DIR`. Use long random JWT and token encryption secrets outside local development. The compose sample leaves MySQL ports commented out; only expose the database for local debugging.
