# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# cache deps
COPY go.mod go.sum ./
RUN go mod download

# build static binaries: the split worker/api processes plus the combined
# tradebot used for local/all-in-one runs.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/tradebot ./cmd/tradebot

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/worker /bin/worker
COPY --from=build /bin/api /bin/api
COPY --from=build /bin/tradebot /bin/tradebot
# fly.toml [processes] selects /bin/worker or /bin/api per machine group.
# CMD is the default for a plain `docker run`; Fly overrides it per process.
CMD ["/bin/tradebot"]
