FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
# Go automatically uses vendor/ when it is present and consistent. Otherwise the
# build stage resolves modules through the configured GOPROXY.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
    -o /out/kube-blackbox ./cmd/kube-blackbox

FROM scratch
COPY --from=build /out/kube-blackbox /kube-blackbox
USER 65532:65532
ENTRYPOINT ["/kube-blackbox"]
