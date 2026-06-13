BINARY         ?= gohtmxelm-demo
PORT           ?= 8091
ELM_SRCS       := $(shell find demo/elm -name '*.elm')
ELM_OUT        := demo/static/elm.js
TEMPL_SRCS     := $(shell find . -name '*.templ' -not -path './.git/*')
TEMPL_OUT      := demo/internal/ui/page_templ.go \
                  demo/internal/ui/components/message_templ.go \
                  demo/internal/ui/components/stopwatch_templ.go \
                  demo/internal/ui/components/fragments_templ.go
HTMX_VERSION   ?= 2.0.4
HTMX_JS        := demo/static/vendor/htmx.js
# Datastar is vendored from a pinned release by default. Override DATASTAR_SRC
# with a local path to copy from disk instead of downloading.
DATASTAR_VERSION ?= 1.0.0-beta.11
DATASTAR_SRC   ?=
DATASTAR_JS    := demo/static/vendor/datastar.js
GO_SRCS        := $(shell find . -name '*.go' -not -path './.git/*')

.PHONY: local build clean test dev watch

## local: build everything then start the server
local: $(BINARY) $(HTMX_JS) $(DATASTAR_JS)
	PORT=$(PORT) ./$(BINARY)

## build: compile generated assets and Go binary
build: $(BINARY)

$(BINARY): go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS) $(GO_SRCS)
	mkdir -p $(dir $@)
	go build -o $@ ./demo

go.sum: go.mod
	go mod tidy

$(HTMX_JS):
	mkdir -p demo/static/vendor
	curl -fsSL https://unpkg.com/htmx.org@$(HTMX_VERSION)/dist/htmx.min.js -o $@

# Vendor Datastar: copy from DATASTAR_SRC when set, otherwise download a pinned
# release so the build is portable across machines.
$(DATASTAR_JS):
	mkdir -p demo/static/vendor
	@if [ -n "$(DATASTAR_SRC)" ]; then \
		echo "copying datastar from $(DATASTAR_SRC)"; \
		cp "$(DATASTAR_SRC)" $@; \
	else \
		echo "downloading datastar@$(DATASTAR_VERSION)"; \
		curl -fsSL https://cdn.jsdelivr.net/gh/starfederation/datastar@v$(DATASTAR_VERSION)/bundles/datastar.js -o $@; \
	fi

# All Elm islands compile into one bundle exposing window.Elm.{AppA,AppB,...},
# which the broker looks up by module name.
$(ELM_OUT): $(ELM_SRCS) demo/elm.json
	cd demo && elm make elm/AppA.elm elm/AppB.elm elm/LapStats.elm elm/RangePicker.elm elm/Simulator.elm --output=static/elm.js

$(TEMPL_OUT): $(TEMPL_SRCS)
	templ generate

## test: run Go tests
test:
	go test ./...

## dev: build generated files and run without compiling a binary
dev: go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS)
	PORT=$(PORT) go run ./demo

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
