BINARY         ?= elm-htmx-templ-demo
PORT           ?= 8091
ELM_OUT        := static/app-a.js static/app-b.js static/lap-stats.js
TEMPL_SRCS     := $(shell find . -name '*.templ' -not -path './.git/*')
TEMPL_OUT      := templates/page_templ.go examples/message_templ.go examples/stopwatch_templ.go
HTMX_JS        := static/vendor/htmx.js
DATASTAR_SRC   ?= /Users/nkhine/go/src/github.com/starfederation/datastar/bundles/datastar.js
DATASTAR_JS    := static/vendor/datastar.js
ONBOARDING_JS  := onboarding/main.js
GO_SRCS        := $(shell find . -name '*.go' -not -path './.git/*')

.PHONY: local build clean test dev watch onboarding

## local: build everything then start the server
local: $(BINARY) $(HTMX_JS) $(DATASTAR_JS)
	./$(BINARY)

## build: compile generated assets and Go binary
build: $(BINARY)

$(BINARY): go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS) $(GO_SRCS)
	mkdir -p $(dir $@)
	go build -o $@ .

go.sum: go.mod
	go mod tidy

$(HTMX_JS):
	mkdir -p static/vendor
	curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o $@

$(DATASTAR_JS): $(DATASTAR_SRC)
	mkdir -p static/vendor
	cp $(DATASTAR_SRC) $@

static/app-a.js: elm/AppA.elm elm/BrokerPort.elm
	elm make elm/AppA.elm --output=$@

static/app-b.js: elm/AppB.elm elm/BrokerPort.elm
	elm make elm/AppB.elm --output=$@

static/lap-stats.js: elm/LapStats.elm elm/BrokerPort.elm
	elm make elm/LapStats.elm --output=$@

$(TEMPL_OUT): $(TEMPL_SRCS)
	templ generate

## test: run Go tests
test:
	go test ./...

## dev: build generated files and run without compiling a binary
dev: go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS)
	go run .

## watch: rebuild generated assets and restart the server on source changes
watch:
	@if lsof -iTCP:$(PORT) -sTCP:LISTEN -n -P >/dev/null 2>&1; then \
		echo "Port $(PORT) is already in use. Stop that process or run: PORT=8092 make watch"; \
		exit 1; \
	fi
	PORT=$(PORT) air -c .air.toml

## onboarding: build the standalone payee onboarding Elm app
onboarding: $(ONBOARDING_JS)

$(ONBOARDING_JS): onboarding/src/Main.elm onboarding/elm.json
	cd onboarding && elm make src/Main.elm --output=main.js

## clean: remove all build artefacts
clean:
	rm -f $(BINARY) $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS) $(ONBOARDING_JS)
