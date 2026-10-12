FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ops-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/ops-service ./ops-service
COPY migrations ./migrations
RUN mkdir -p /data/private
ENV STORAGE_ROOT=/data/private
EXPOSE 8093
ENTRYPOINT ["./ops-service"]
