.PHONY: build test test-integration lint run stop logs demo

GO     := PATH=$$PATH:/usr/local/go/bin go
DOCKER := docker compose -f deploy/docker-compose.yml

build:
	$(GO) build ./...

test:
	$(GO) test ./... -race -count=1

test-integration:
	FORGE_DATABASE_URL=postgres://forge:forge@localhost:5432/forge?sslmode=disable \
	$(GO) test ./... -count=1 -v

lint:
	$(GO) vet ./...

run:
	$(DOCKER) up --build -d

stop:
	$(DOCKER) down -v

logs:
	$(DOCKER) logs -f forge-api forge-worker

demo:
	@bash scripts/demo.sh
