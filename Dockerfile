# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure-Go (modernc sqlite) => CGO disabled => fully static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /glimt ./cmd/glimt

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /data
COPY --from=build /glimt /glimt

ENV GLIMT_DB=/data/glimt.db \
    GLIMT_ADDR=:8080
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/glimt"]
