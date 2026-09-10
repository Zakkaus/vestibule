FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates \
 && addgroup -S -g 65532 vestibule \
 && adduser -S -D -H -u 65532 -G vestibule vestibule \
 && mkdir -p /usr/share/doc/vestibule \
 && cp /lib/apk/db/installed /tmp/runtime-apk-installed

FROM runtime AS runtime-notices
RUN apk add --no-cache abuild python3 wget
COPY THIRD-PARTY-LICENSES /tmp/THIRD-PARTY-LICENSES
COPY scripts/bundle-alpine-runtime-sources.py /usr/local/bin/bundle-alpine-runtime-sources.py
RUN python3 /usr/local/bin/bundle-alpine-runtime-sources.py \
      --installed /tmp/runtime-apk-installed \
      --output /out/alpine-runtime-sources \
      --base-artifact /tmp/THIRD-PARTY-LICENSES \
      --artifact /out/THIRD-PARTY-LICENSES

FROM golang:1.26.7-alpine AS build

ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/vestibule ./cmd/bot

FROM runtime
COPY --from=build /out/vestibule /usr/local/bin/vestibule
COPY --from=runtime-notices /out/THIRD-PARTY-LICENSES /usr/share/doc/vestibule/THIRD-PARTY-LICENSES
COPY --from=runtime-notices /out/alpine-runtime-sources /usr/share/doc/vestibule/alpine-runtime-sources
USER vestibule:vestibule
ENTRYPOINT ["/usr/local/bin/vestibule"]
