FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mode-demo ./examples/modes

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /mode-demo /mode-demo
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=3s --start-period=5s --retries=15 CMD ["/mode-demo", "-healthcheck"]
ENTRYPOINT ["/mode-demo"]
