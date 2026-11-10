FROM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The release workflow rewrites version.json before it builds and passes no
# build args, so the version always comes from the tree.
RUN VERSION="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' version.json)" \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/error-pages ./cmd/error-pages \
 && mkdir -p /out/data

FROM scratch
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build /out/error-pages /error-pages
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/error-pages","healthcheck"]
ENTRYPOINT ["/error-pages"]
CMD ["serve"]
