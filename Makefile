BINARY  := elm-htmx-templ-demo
ELM_OUT := static/app-a.js static/app-b.js
TEMPL_OUT := templates/page_templ.go

.PHONY: local build clean

## local: build everything then start the server
local: build
	./$(BINARY)

## build: compile Elm, generate templ, then build the Go binary
build: go.sum $(ELM_OUT) $(TEMPL_OUT)
	go build -o $(BINARY) .

go.sum: go.mod
	go mod tidy

static/app-a.js: elm/AppA.elm
	elm make $< --output=$@

static/app-b.js: elm/AppB.elm
	elm make $< --output=$@

$(TEMPL_OUT): templates/page.templ
	templ generate

## clean: remove all build artefacts
clean:
	rm -f $(BINARY) $(ELM_OUT) $(TEMPL_OUT)
