FROM golang:1.15-alpine AS BUILDER

WORKDIR /build

# cache the module downloads
COPY ./src/go.mod .
COPY ./src/go.sum .
RUN go mod download

# build our application
COPY ./src .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o ./bin/esp-updater

# start from scratch and just copy the related stuff
FROM scratch

COPY --from=BUILDER /build/bin/ /

ENTRYPOINT ["/esp-updater"]