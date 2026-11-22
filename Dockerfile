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

# mihomo (GPL-3.0, https://github.com/MetaCubeX/mihomo) is shipped as a separate,
# unmodified program; cusconf runs it for node tests. Its license is included below.
FROM alpine:3.22 AS mihomo
ARG TARGETARCH
ARG MIHOMO_VERSION=v1.19.34
RUN apk add --no-cache curl && \
    curl -fsSL "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-${TARGETARCH}-${MIHOMO_VERSION}.gz" | gunzip > /mihomo && \
    chmod +x /mihomo && \
    curl -fsSL -o /mihomo-LICENSE "https://raw.githubusercontent.com/MetaCubeX/mihomo/${MIHOMO_VERSION}/LICENSE"

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 1000 cusconf
COPY --from=mihomo /mihomo /usr/local/bin/mihomo
COPY --from=mihomo /mihomo-LICENSE /usr/share/licenses/mihomo/LICENSE
COPY LICENSE NOTICE /usr/share/licenses/cusconf/
USER cusconf
COPY --from=build /out/cusconf /usr/local/bin/cusconf
ENV CUSCONF_DATA=/data CUSCONF_ADDR=:8080 CUSCONF_MIHOMO=/usr/local/bin/mihomo
VOLUME /data
EXPOSE 8080
HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["cusconf"]
