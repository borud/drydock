ifeq ($(GOPATH),)
GOPATH := $(HOME)/go
endif

all: test lint vet build

clean:
	@rm -f bin/*
	@go clean -testcache

build: dock

dock:
	@cd cmd/dock && go build -o ../../bin/dock

check: lint vet staticcheck revive

lint:
	@revive

vet:
	@go vet ./...

staticcheck:
	@staticcheck ./...

revive:
	@revive ./...

test:
	@go test ./...

test_verbose:
	@go test ./... -v

test_race:
	@go test ./... -race

benchmark:
	@go test -bench .

count:
	@echo "Linecounts excluding generated and third party code"
	@gocloc --not-match-d='apipb|openapi|third_party' .
