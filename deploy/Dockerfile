FROM golang:1.26.7-alpine AS build

ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/vestibule ./cmd/bot

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
 && addgroup -S -g 65532 vestibule \
 && adduser -S -D -H -u 65532 -G vestibule vestibule
COPY --from=build /out/vestibule /usr/local/bin/vestibule
USER vestibule:vestibule
ENTRYPOINT ["/usr/local/bin/vestibule"]
