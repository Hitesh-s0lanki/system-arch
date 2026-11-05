FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/load-tester ./cmd/server

FROM docker:28.2.2-cli
RUN apk add --no-cache ca-certificates
COPY --from=build /out/load-tester /usr/local/bin/load-tester
ENV DATA_DIR=/data LISTEN_ADDR=:8090
EXPOSE 8090
ENTRYPOINT ["load-tester"]
