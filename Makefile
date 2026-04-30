.PHONY: help build up down logs ps restart test test-e2e curl-info curl-health

COMPOSE := docker compose

# Run unit tests inside a throwaway golang container so the host
# stays toolchain-free. Mirrors how the operator builds.
GO_TEST := docker run --rm -v "$(CURDIR)":/src -w /src golang:1.22-alpine

help:
	@echo "Targets:"
	@echo "  build        - build the edge image"
	@echo "  up           - start edge in the background"
	@echo "  down         - stop edge"
	@echo "  logs         - tail edge logs"
	@echo "  ps           - container status"
	@echo "  restart      - down + up"
	@echo "  test         - run unit tests in a golang container"
	@echo "  test-e2e     - run e2e tests against running edge+operator"
	@echo "  curl-health  - GET /v1/health on the running edge"
	@echo "  curl-info    - GET /v1/info  on the running edge"

build:
	$(COMPOSE) build

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

restart: down up

test:
	$(GO_TEST) sh -c "go mod tidy && go test ./..."

test-e2e:
	docker run --rm \
	  -v "$(CURDIR)":/src -w /src \
	  -e EDGE_URL=http://host.docker.internal:8081 \
	  --add-host=host.docker.internal:host-gateway \
	  golang:1.22-alpine sh -c "go mod tidy && go test -tags=e2e -count=1 ./e2e/..."

curl-health:
	@curl -fsS http://localhost:8081/v1/health | jq .

curl-info:
	@curl -fsS http://localhost:8081/v1/info | jq .
