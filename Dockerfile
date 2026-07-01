# fulcrum Go core -> scratch image. The SPA is embedded from the committed
# web/dist placeholder; once the real Vite build lands, add a node stage that
# writes to web/dist before this build.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build \
    -trimpath -ldflags="-s -w -X github.com/t0mer/fulcrum/internal/version.Version=${VERSION}" \
    -o /out/fulcrum ./cmd/fulcrum
# Pre-create the data dir so the scratch image has a writable mount point.
RUN mkdir -p /data

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/fulcrum /fulcrum
COPY --from=builder /data /data
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/fulcrum"]
