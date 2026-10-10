# Atajos de desarrollo. Cada servicio es un binario independiente.
SERVICES := gateway presentations realtime rooms
GO       ?= go

.PHONY: run build test test-short test-race vet fmt clean $(addprefix run-,$(SERVICES))

## run: arranca todos los servicios en paralelo (Ctrl+C los detiene todos)
run:
	@$(MAKE) -j$(words $(SERVICES)) --no-print-directory $(addprefix run-,$(SERVICES))

## run-<servicio>: arranca un único servicio
run-gateway:
	$(GO) run ./services/gateway

run-presentations:
	$(GO) run ./services/presentations

run-realtime:
	$(GO) run ./services/realtime

run-rooms:
	$(GO) run ./services/rooms

## build: compila los binarios en bin/
build:
	@mkdir -p bin
	@for s in $(SERVICES); do $(GO) build -o bin/$$s ./services/$$s || exit 1; done
	@echo "binarios en bin/: $(SERVICES)"

## test: todos los tests (incluye integración con LibreOffice y pdftoppm si están instalados, y con
## PostgreSQL si se define TEST_DATABASE_URL)
test:
	$(GO) test ./...

## test-short: tests unitarios, sin LibreOffice, pdftoppm ni PostgreSQL
test-short:
	$(GO) test -short ./...

## test-race: tests con el detector de carreras
test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
