# Atajos de desarrollo. Cada servicio es un binario independiente.
SERVICES := gateway presentations realtime
GO       ?= go

.PHONY: run build test test-short test-race vet fmt clean $(addprefix run-,$(SERVICES))

## run: arranca los tres servicios en paralelo (Ctrl+C los detiene todos)
run:
	@$(MAKE) -j3 --no-print-directory $(addprefix run-,$(SERVICES))

## run-<servicio>: arranca un único servicio
run-gateway:
	$(GO) run ./services/gateway

run-presentations:
	$(GO) run ./services/presentations

run-realtime:
	$(GO) run ./services/realtime

## build: compila los binarios en bin/
build:
	@mkdir -p bin
	@for s in $(SERVICES); do $(GO) build -o bin/$$s ./services/$$s || exit 1; done
	@echo "binarios en bin/: $(SERVICES)"

## test: todos los tests (incluye integración con LibreOffice si está instalado)
test:
	$(GO) test ./...

## test-short: tests unitarios, sin LibreOffice
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
