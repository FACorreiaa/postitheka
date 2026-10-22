# Web stage: the composer, built to static files for the Go binary to embed.
FROM node:26.12.1-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

# Build stage.
FROM golang:1.27.3 AS build
WORKDIR /app

# Dependency files first, so the download layer is cached across code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /web/dist ./web/dist

# Generated code is committed, so the image builds from the tree as checked
# in; `task check` is what keeps it current.
#
# Static binary: the schema (goose SQL and River's migrations) is embedded, so
# the runtime image needs nothing beside it. `server migrate` applies it.
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Runtime stage.
FROM alpine:3.20.2
WORKDIR /app

# Certificates for outbound TLS to Discord, X, Meta and R2.
RUN apk add --no-cache ca-certificates

ENV GO_ENV=production

COPY --from=build /app/server .

EXPOSE 8095

# ENTRYPOINT rather than CMD so the deployment can pass a subcommand as `args`
# without replacing the binary: the migration hook runs `server migrate`.
ENTRYPOINT ["/app/server"]
CMD []
