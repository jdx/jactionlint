ARG GOLANG_VER=latest
ARG ALPINE_VER=latest

FROM golang:${GOLANG_VER} AS builder
WORKDIR /go/src/app
COPY go.* *.go ./
COPY cmd cmd/
COPY internal internal/
ENV CGO_ENABLED=0
ARG JACTIONLINT_VER=
RUN go build -v -ldflags "-s -w -X github.com/jdx/jactionlint/v2.version=${JACTIONLINT_VER}" ./cmd/jactionlint

FROM koalaman/shellcheck-alpine:stable AS shellcheck

FROM alpine:${ALPINE_VER}
COPY --from=builder /go/src/app/jactionlint /usr/local/bin/
COPY --from=shellcheck /bin/shellcheck /usr/local/bin/shellcheck
RUN apk add --no-cache py3-pyflakes
USER guest
ENTRYPOINT ["/usr/local/bin/jactionlint"]
