# Builder
FROM golang:1.20-alpine3.18 as builder

ARG VERSION="0.0.0-build"
ENV VERSION=$VERSION

WORKDIR /go/src/app

RUN apk add --no-cache \
  bash build-base git make

# Copy module files and download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN make build

# Final
FROM alpine:3.18
RUN apk upgrade && apk add --no-cache bash curl

RUN addgroup -g 1001 app
RUN adduser -D -G app -u 1001 app

COPY --from=builder /go/src/app/build/cosmos-validator-watcher /

WORKDIR /
ENTRYPOINT ["/cosmos-validator-watcher"]
