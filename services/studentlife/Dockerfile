FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/studentlife-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/studentlife-service ./studentlife-service
COPY migrations ./migrations
EXPOSE 8094
ENTRYPOINT ["./studentlife-service"]
