FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o api main.go

FROM scratch

COPY --from=builder /build/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]
