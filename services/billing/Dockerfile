FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/billing-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/billing-service ./billing-service
COPY migrations ./migrations
# Private transfer-proof storage. In production this is a mounted persistent
# volume (see the root docker-compose.yml `billing_files`), never the container.
RUN mkdir -p /data/private
ENV STORAGE_ROOT=/data/private
EXPOSE 8090
ENTRYPOINT ["./billing-service"]
