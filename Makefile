BINARY         ?= gohtmxelm-demo
PORT           ?= 8091
# TLS=1 serves the dev server over HTTP/2 (self-signed localhost cert) so the
# browser multiplexes every SSE stream over one connection instead of hitting
# the HTTP/1.1 ~6-connections-per-host limit. Set TLS=0 for plain HTTP.
TLS            ?= 1
STARBASE_DIR   ?= /Users/nkhine/go/src/github.com/Shieldpay/starbase
FLOCI_ENDPOINT ?= http://localhost:4566
EDGE_LAMBDA_ZIP := dist/edge-datastar-lambda.zip
ELM_SRCS       := $(shell find demo/elm -name '*.elm')
ELM_OUT        := demo/static/elm.js
TEMPL_SRCS     := $(shell find . -name '*.templ' -not -path './.git/*')
TEMPL_OUT      := demo/internal/ui/page_templ.go \
                  demo/internal/ui/components/message_templ.go \
                  demo/internal/ui/components/stopwatch_templ.go \
                  demo/internal/ui/components/fragments_templ.go \
                  demo/internal/ui/components/localization_templ.go \
                  demo/internal/ui/components/edge_datastar_templ.go
HTMX_VERSION   ?= 2.0.4
HTMX_JS        := demo/static/vendor/htmx.js
# Datastar is vendored from a pinned release by default. Override DATASTAR_SRC
# with a local path to copy from disk instead of downloading.
DATASTAR_VERSION ?= 1.0.0-beta.11
DATASTAR_SRC   ?=
DATASTAR_JS    := demo/static/vendor/datastar.js
GO_SRCS        := $(shell find . -name '*.go' -not -path './.git/*')

.PHONY: local build clean test dev watch package-edge-lambda floci-preflight edge-infra-up edge-enable-streaming edge-infra-down edge-infra-output

## local: build everything then start the server
local: $(BINARY) $(HTMX_JS) $(DATASTAR_JS)
	GOHTMXELM_TLS=$(TLS) PORT=$(PORT) ./$(BINARY)

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
	cd demo && elm make elm/AppA.elm elm/AppB.elm elm/LapStats.elm elm/RangePicker.elm elm/Simulator.elm elm/LocaleEcho.elm elm/AuthPresence.elm --output=static/elm.js

$(TEMPL_OUT): $(TEMPL_SRCS)
	templ generate

## test: run Go tests
test:
	go test ./...

## dev: build generated files and run without compiling a binary
dev: go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS)
	GOHTMXELM_TLS=$(TLS) PORT=$(PORT) go run ./demo

## watch: rebuild generated assets and restart the server on source changes
watch:
	@if lsof -iTCP:$(PORT) -sTCP:LISTEN -n -P >/dev/null 2>&1; then \
		echo "Port $(PORT) is already in use. Stop that process or run: PORT=8092 make watch"; \
		exit 1; \
	fi
	GOHTMXELM_TLS=$(TLS) PORT=$(PORT) air -c .air.toml

## package-edge-lambda: build the Datastar SSE Lambda bootstrap zip
package-edge-lambda: go.sum
	mkdir -p dist
	rm -f dist/bootstrap $(EDGE_LAMBDA_ZIP)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dist/bootstrap ./cmd/edge-datastar-apigw-lambda
	cd dist && zip -q edge-datastar-lambda.zip bootstrap
	rm -f dist/bootstrap

## floci-preflight: ensure the Starbase floci edge is listening
floci-preflight:
	@if curl -fsS "$(FLOCI_ENDPOINT)/_localstack/health" >/dev/null 2>&1; then \
		echo "floci is available at $(FLOCI_ENDPOINT)"; \
	elif [ -d "$(STARBASE_DIR)" ]; then \
		echo "starting floci via $(STARBASE_DIR)"; \
		$(MAKE) -C "$(STARBASE_DIR)" ddb-up; \
	else \
		echo "floci is not reachable at $(FLOCI_ENDPOINT), and STARBASE_DIR=$(STARBASE_DIR) does not exist"; \
		exit 1; \
	fi

## edge-infra-up: deploy the local API Gateway + Lambda streaming demo to floci
edge-infra-up: package-edge-lambda floci-preflight
	cd infra && (pulumi stack select local || pulumi stack init local)
	cd infra && pulumi up --yes --stack local

## edge-enable-streaming: patch API Gateway responseTransferMode=STREAM
edge-enable-streaming:
	cd infra && cmd=$$(pulumi stack output edge:streamingPatchCommand --stack local); echo "$$cmd"; case "$$cmd" in AWS_ACCESS_KEY_ID=*) eval "$$cmd";; *) true;; esac

## edge-infra-output: print the local edge stack outputs
edge-infra-output:
	cd infra && pulumi stack output --stack local

## edge-infra-down: destroy the local edge stack
edge-infra-down:
	cd infra && pulumi destroy --yes --stack local

## clean: remove all build artefacts
clean:
	rm -f $(BINARY) $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS) $(DATASTAR_JS) $(EDGE_LAMBDA_ZIP) dist/bootstrap
