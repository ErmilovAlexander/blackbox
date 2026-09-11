.PHONY: build test test-short vet fmt fmt-check verify vendor image image-push chart-lint chart-package chart-push release-verify manifests install smoke clean

GO ?= go
BINARY ?= bin/kube-blackbox
IMAGE ?= kube-blackbox:dev
VERSION ?= dev
PLATFORM ?= linux/amd64
HELM ?= helm
CHART ?= charts/kube-blackbox
CHART_DEST ?= dist
CHART_VERSION ?= 0.1.3
CHART_REPOSITORY ?= https://mirror.ip-10-28-32-189.shturval.link/repository/shturval_helm/
NEXUS_USER ?= admin
NEXUS_TLS_FLAGS ?= --insecure
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

verify: fmt-check vet test build manifests chart-lint

# Run once in a connected build environment, then carry vendor/ into an air-gapped build zone.
vendor:
	$(GO) mod tidy
	$(GO) mod vendor

image:
	docker buildx build --platform $(PLATFORM) --load --build-arg VERSION=$(VERSION) -t $(IMAGE) .

image-push:
	docker buildx build --platform $(PLATFORM) --push --build-arg VERSION=$(VERSION) -t $(IMAGE) .

chart-lint:
	$(HELM) lint $(CHART)
	$(HELM) template kube-blackbox $(CHART) --namespace kube-blackbox >/dev/null

chart-package: chart-lint
	mkdir -p $(CHART_DEST)
	$(HELM) package $(CHART) --destination $(CHART_DEST)

chart-push: chart-package
	curl --fail-with-body $(NEXUS_TLS_FLAGS) --user $(NEXUS_USER) \
		--upload-file $(CHART_DEST)/kube-blackbox-$(CHART_VERSION).tgz \
		$(CHART_REPOSITORY)

release-verify:
	./hack/verify-nexus-release.sh

manifests:
	kubectl kustomize deploy >/dev/null

install:
	kubectl apply -k deploy

smoke:
	./hack/kubernetes-smoke-test.sh

clean:
	rm -rf bin dist
