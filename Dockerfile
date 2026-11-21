# Place Space as a server for a home network:
#   docker run -p 8080:8080 -v place-space:/data ghcr.io/eggs-gd/place-space
# Each person picks a profile in the browser; see README.

FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# pack-ui writes the built pages into internal/ui/dist.
RUN mkdir -p ../internal/ui && npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS go
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags ui -trimpath -ldflags "-s -w" -o /out/placespace ./cmd/placespace
# The image runs as nonroot; a fresh volume copies this directory's owner.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/placespace /usr/local/bin/placespace
COPY --from=go --chown=nonroot:nonroot /out/data /data
ENV PLACE_SPACE_DATA=/data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["placespace", "health"]
ENTRYPOINT ["placespace"]
CMD ["serve", "-addr", ":8080", "-profiles"]
