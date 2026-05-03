# Build stage — always runs on the host platform; cross-compiles for the target.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /src

# Download dependencies in a separate layer so they are cached across code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TARGETARCH is injected by BuildKit for the requested platform.
# TARGETOS is always linux — this exporter uses Linux netlink and will not
# compile for any other OS.
ARG TARGETARCH

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w \
        -X main.version=${VERSION} \
        -X main.commit=${COMMIT} \
        -X main.builtAt=${BUILT_AT}" \
      -o /qdisc-exporter \
      .


# Runtime stage — scratch keeps the image as small as possible.
# The binary is fully static (CGO_ENABLED=0), so no libc is needed.
FROM scratch

COPY --from=builder /qdisc-exporter /qdisc-exporter

EXPOSE 9700

# The exporter must run in the host network namespace to see the host's qdiscs:
#   docker run --network=host ghcr.io/spijet/qdisc-exporter
ENTRYPOINT ["/qdisc-exporter"]
CMD ["--listen", ":9700"]
