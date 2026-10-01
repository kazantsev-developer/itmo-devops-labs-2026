FROM golang:1.27-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /build/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /build/worker ./cmd/worker


FROM scratch AS api

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]


FROM scratch AS worker

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/worker /worker

EXPOSE 8081

ENTRYPOINT ["/worker"]