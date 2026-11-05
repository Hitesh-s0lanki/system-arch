# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/ratelimitd ./cmd/ratelimitd \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/demo-backend ./examples/backend

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ratelimitd /ratelimitd
COPY --from=build /out/demo-backend /demo-backend
COPY configs/proxy.json /etc/ratelimit/proxy.json
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=5 \
    CMD ["/ratelimitd", "-healthcheck", "http://127.0.0.1:8080/readyz"]
ENTRYPOINT ["/ratelimitd"]
CMD ["-config", "/etc/ratelimit/proxy.json"]
