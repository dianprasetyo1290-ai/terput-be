FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/admission-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/admission-service ./admission-service
COPY migrations ./migrations
# Private upload storage. In production this is a mounted persistent volume
# (see the root docker-compose.yml `admission_files`), never the container.
RUN mkdir -p /data/private
ENV STORAGE_ROOT=/data/private
EXPOSE 8088
ENTRYPOINT ["./admission-service"]
