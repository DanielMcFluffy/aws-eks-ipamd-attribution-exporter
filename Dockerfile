FROM --platform=$BUILDPLATFORM golang:1.26 AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/ipamd-attribution-exporter ./cmd/ipamd-attribution-exporter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/ipamd-attribution-exporter /ipamd-attribution-exporter
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/ipamd-attribution-exporter"]
