FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY api/go.mod ./
RUN go mod download

COPY api/main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o api main.go

FROM scratch

COPY --from=builder /build/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]
