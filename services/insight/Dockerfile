FROM golang:1.26.4-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /insight-server ./cmd/server

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /insight-server /app/insight-server

EXPOSE 8096
CMD ["/app/insight-server"]
