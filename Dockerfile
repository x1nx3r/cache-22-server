FROM golang:1.25-bookworm AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /server /app/server
COPY --from=build /app/static /app/static
EXPOSE 8080
VOLUME ["/data"]
ENV DB_URL=postgres://cache22:cache22@postgres:5432/cache22?sslmode=disable LIBRARY_PATH=/data/library HTTP_PORT=8080
ENTRYPOINT ["/app/server"]
