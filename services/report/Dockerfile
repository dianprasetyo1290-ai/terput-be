FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/report-service ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/report-migrate ./cmd/migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/report-service ./report-service
COPY --from=build /out/report-migrate ./report-migrate
COPY migrations ./migrations
EXPOSE 8086
ENTRYPOINT ["./report-service"]
