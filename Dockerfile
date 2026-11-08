# syntax=docker/dockerfile:1
# routerctl: a static binary on distroless, running as non-root.
# The build context is the repository root (go.work and its modules).
#
#   docker build -t routerctl .
#   docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t routerctl .

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
WORKDIR /src
# Workspace and module files first, so the download layer is cached until
# a go.mod or go.sum changes (the globs also match modules without go.sum)
COPY go.work go.work.sum ./
COPY netkit/go.* netkit/
COPY asusrouter/go.* asusrouter/
COPY routerkit/go.* routerkit/
COPY routerctl/go.* routerctl/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/routerctl ./routerctl/cmd/routerctl \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="routerctl" \
      org.opencontainers.image.description="Multi-vendor home-router CLI and daemon (ASUS, ZTE). Includes a Go port of the asusrouter Python library by Yevhenii Vaskivskyi." \
      org.opencontainers.image.source="https://github.com/eduardpaul/routerctl" \
      org.opencontainers.image.url="https://github.com/eduardpaul/routerctl" \
      org.opencontainers.image.documentation="https://github.com/eduardpaul/routerctl#readme" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.authors="Eduard Paul" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
COPY LICENSE NOTICE /usr/share/doc/routerctl/
COPY --from=build /out/routerctl /usr/local/bin/routerctl
# State file (device inventory) lives in /data: mount a volume there
COPY --from=build --chown=65532:65532 /out/data /data
ENV ROUTERCTL_STATE_FILE=/data/routerctl-state.json
# Listen on every interface inside the container; with the API enabled
# this requires a token (ROUTERCTL_HTTP_TOKEN or http.token_file)
ENV ROUTERCTL_HTTP_LISTEN=:9100
VOLUME ["/data"]
USER 65532:65532
# /healthz, /metrics and /api/v1 (when metrics or the API are enabled)
EXPOSE 9100
ENTRYPOINT ["/usr/local/bin/routerctl"]
CMD ["watch"]
