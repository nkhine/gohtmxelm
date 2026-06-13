BINARY         ?= gohtmxelm-demo
PORT           ?= 8091
ELM_OUT        := demo/static/app-a.js demo/static/app-b.js demo/static/lap-stats.js
TEMPL_SRCS     := $(shell find . -name '*.templ' -not -path './.git/*')
TEMPL_OUT      := demo/internal/ui/page_templ.go demo/internal/ui/components/message_templ.go demo/internal/ui/components/stopwatch_templ.go
HTMX_JS        := demo/static/vendor/htmx.js
DATASTAR_SRC   ?= /Users/nkhine/go/src/github.com/starfederation/datastar/bundles/datastar.js
DATASTAR_JS    := demo/static/vendor/datastar.js
GO_SRCS        := $(shell find . -name '*.go' -not -path './.git/*')

.PHONY: local build clean test dev watch

## local: build everything then start the server
local: $(BINARY) $(HTMX_JS) $(DATASTAR_JS)
	./$(BINARY)

## build: compile generated assets and Go binary
build: $(BINARY)

$(BINARY): go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS) $(GO_SRCS)
	mkdir -p $(dir $@)
	go build -o $@ ./demo

go.sum: go.mod
	go mod tidy

$(HTMX_JS):
	mkdir -p demo/static/vendor
	curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o $@

$(DATASTAR_JS): $(DATASTAR_SRC)
	mkdir -p demo/static/vendor
	cp $(DATASTAR_SRC) $@

demo/static/app-a.js: demo/elm/AppA.elm demo/elm/BrokerPort.elm demo/elm.json
	cd demo && elm make elm/AppA.elm --output=static/app-a.js

demo/static/app-b.js: demo/elm/AppB.elm demo/elm/BrokerPort.elm demo/elm.json
	cd demo && elm make elm/AppB.elm --output=static/app-b.js

demo/static/lap-stats.js: demo/elm/LapStats.elm demo/elm/BrokerPort.elm demo/elm.json
	cd demo && elm make elm/LapStats.elm --output=static/lap-stats.js

$(TEMPL_OUT): $(TEMPL_SRCS)
	templ generate

## test: run Go tests
test:
	go test ./...

## dev: build generated files and run without compiling a binary
dev: go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS)
	go run ./demo

## watch: rebuild generated assets and restart the server on source changes
watch:
	@if lsof -iTCP:$(PORT) -sTCP:LISTEN -n -P >/dev/null 2>&1; then \
		echo "Port $(PORT) is already in use. Stop that process or run: PORT=8092 make watch"; \
		exit 1; \
	fi
	PORT=$(PORT) air -c .air.toml

## clean: remove all build artefacts
clean:
	rm -f $(BINARY) $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS)
