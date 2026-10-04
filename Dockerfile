FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go build -o /aimangebot .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git && rm -rf /var/lib/apt/lists/* && useradd -m -u 10001 app
WORKDIR /app
COPY --from=backend /aimangebot /usr/local/bin/aimangebot
COPY config.example.yaml /app/config.example.yaml
RUN chown app:app /app
USER app
EXPOSE 8080
ENTRYPOINT ["aimangebot"]
