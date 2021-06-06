ifeq ($(GOPATH),)
GOPATH := $(HOME)/go
endif

all: test lint vet staticcheck

clean:
	@rm -f bin/*
	@go clean -testcache

check: lint vet staticcheck revive

vet:
	@go vet ./...

staticcheck:
	@staticcheck ./...

lint:
	@revive ./...

test:
	@go test ./...

count:
	@echo "Linecounts excluding generated and third party code"
	@gocloc --not-match-d='apipb|openapi|third_party' .
