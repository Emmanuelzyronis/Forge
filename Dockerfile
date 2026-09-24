FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o forge-api ./cmd/forge-api

# Download the migrate binary for the target arch
FROM alpine:3.20 AS migrate-dl
RUN apk add --no-cache curl tar
RUN curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz \
    | tar -xz -C /tmp && mv /tmp/migrate /migrate

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/forge-api .
COPY --from=migrate-dl /migrate /usr/local/bin/migrate
COPY migrations/ ./migrations/
COPY start.sh ./start.sh
RUN chmod +x ./start.sh
EXPOSE 8080
CMD ["./start.sh"]
