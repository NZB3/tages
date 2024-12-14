FROM golang:1.23.1-alpine AS builder

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main ./cmd/

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 9000

CMD ["./main"]