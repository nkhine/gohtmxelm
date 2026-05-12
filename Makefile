BINARY    := elm-htmx-templ-demo
ELM_OUT   := static/app-a.js static/app-b.js
TEMPL_OUT := templates/page_templ.go
HTMX_JS   := static/vendor/htmx.js
GO_SRCS   := $(shell find . -name '*.go' -not -path './.git/*')

.PHONY: local clean test dev

## local: build everything then start the server
local: $(BINARY) $(HTMX_JS)
	./$(BINARY)

$(BINARY): go.sum $(ELM_OUT) $(TEMPL_OUT) $(GO_SRCS)
	go build -o $@ .

go.sum: go.mod
	go mod tidy

$(HTMX_JS):
	mkdir -p static/vendor
	curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o $@

static/app-a.js: elm/AppA.elm elm/BrokerPort.elm
	elm make elm/AppA.elm --output=$@

static/app-b.js: elm/AppB.elm elm/BrokerPort.elm
	elm make elm/AppB.elm --output=$@

$(TEMPL_OUT): templates/page.templ
	templ generate

## test: run Go tests
test:
	go test ./...

## dev: build generated files and run without compiling a binary
dev: go.sum $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS)
	go run .

## clean: remove all build artefacts
clean:
	rm -f $(BINARY) $(ELM_OUT) $(TEMPL_OUT) $(HTMX_JS)
