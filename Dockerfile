# syntax=docker/dockerfile:1
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cusconf ./cmd/cusconf

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 1000 cusconf
USER cusconf
COPY --from=build /out/cusconf /usr/local/bin/cusconf
ENV CUSCONF_DATA=/data CUSCONF_ADDR=:8080
VOLUME /data
EXPOSE 8080
HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["cusconf"]
