# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dns-management .

FROM alpine:3.22
WORKDIR /app
RUN addgroup -S app && adduser -S app -G app
COPY --from=backend-build /out/dns-management /app/dns-management
COPY --from=frontend-build /src/frontend/out /app/public
ENV ADDR=:8080
ENV FRONTEND_DIR=/app/public
EXPOSE 8080
USER app
ENTRYPOINT ["/app/dns-management"]
