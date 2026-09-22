# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/amd64/github-merger ./cmd/github-merger && \
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/arm64/github-merger ./cmd/github-merger

FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH
COPY --from=build --chown=65532:65532 /out/${TARGETARCH}/github-merger /github-merger
USER 65532:65532
ENTRYPOINT ["/github-merger"]
