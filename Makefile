.PHONY: build test test-short vet fmt fmt-check verify vendor image manifests install smoke clean

GO ?= go
BINARY ?= bin/kube-blackbox
IMAGE ?= kube-blackbox:dev
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	mkdir -p $(dir $(BINARY))
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(BINARY) ./cmd/kube-blackbox

test:
	$(GO) test -race ./...

test-short:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f -not -path './vendor/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -type f -not -path './vendor/*'))" || \
		(printf '%s\n' 'Go files are not formatted; run make fmt' && exit 1)

verify: fmt-check vet test build manifests

# Run once in a connected build environment, then carry vendor/ into an air-gapped build zone.
vendor:
	$(GO) mod tidy
	$(GO) mod vendor

image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

manifests:
	kubectl kustomize deploy >/dev/null

install:
	kubectl apply -k deploy

smoke:
	./hack/kubernetes-smoke-test.sh

clean:
	rm -rf bin
