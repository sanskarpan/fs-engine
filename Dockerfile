# syntax=docker/dockerfile:1.7

FROM golang:1.26.3 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/fs-engine ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/fs-engine /app/fs-engine

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/app/fs-engine"]
CMD ["-addr", ":8080", "-img", "/data/disk.img"]
