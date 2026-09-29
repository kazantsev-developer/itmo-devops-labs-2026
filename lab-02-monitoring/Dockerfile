FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY main.go telemetry.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o api .

FROM scratch

COPY --from=builder /build/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]