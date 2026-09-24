.PHONY: build test lint docker-up docker-down migrate-up migrate-down docker-logs

GO     := PATH=$$PATH:/usr/local/go/bin go
DOCKER := docker compose -f deploy/docker-compose.yml

build:
	$(GO) build ./...

test:
	$(GO) test ./... -race -count=1

lint:
	$(GO) vet ./...

migrate-up:
	@echo "Migrations run automatically on forge-api startup."
	@echo "Layer 3 adds the authoritative PostgreSQL schema."

migrate-down:
	@echo "Layer 3 adds schema migration rollback."

docker-up:
	$(DOCKER) up -d

docker-down:
	$(DOCKER) down -v

docker-logs:
	$(DOCKER) logs -f
