# syntax=docker/dockerfile:1

# keycloak-doctor depends on nothing outside the standard library, so the build
# needs no module download and the image needs no runtime: a static binary, the
# root certificates the Admin REST API needs over HTTPS, and nothing else. An
# image that audits a production SSO server should not also carry a shell, a
# package manager and a libc for someone to find.

FROM golang:1.25-alpine AS build

# ca-certificates is here for its bundle, which is copied into the final image;
# the build itself fetches nothing.
RUN apk add --no-cache ca-certificates

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/keycloak-doctor ./cmd/keycloak-doctor

FROM scratch

ARG VERSION=dev
LABEL org.opencontainers.image.title="keycloak-doctor" \
      org.opencontainers.image.description="Audit a Keycloak realm for the mistakes that actually get exploited — from an export file or a live server." \
      org.opencontainers.image.source="https://github.com/Allan-Nava/keycloak-doctor" \
      org.opencontainers.image.documentation="https://allan-nava.github.io/keycloak-doctor/" \
      org.opencontainers.image.licenses="LicenseRef-PolyForm-Noncommercial-1.0.0" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/keycloak-doctor /keycloak-doctor

# Mount the export read-only here: the tool never writes to its input, and with
# --out-file it writes only where you point it.
WORKDIR /realm

# Numeric, because a scratch image has no /etc/passwd to name a user in. Nothing
# the audit does needs root, or a writable filesystem.
USER 65532:65532

ENTRYPOINT ["/keycloak-doctor"]
CMD ["--help"]
