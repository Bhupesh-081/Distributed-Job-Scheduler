FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
# SERVICE picks which cmd/ binary this image runs — api, job-service,
# watcher-service, or consumer-service all build from this one Dockerfile.
ARG SERVICE=api
RUN CGO_ENABLED=0 go build -o /out/service ./cmd/${SERVICE}

FROM alpine:3.20
# python3 is here so consumer-service can actually run the "Python script"
# job type from the dashboard; harmless (tiny) on the other 3 services that
# share this image but never invoke it.
RUN apk add --no-cache python3 && adduser -D -H app
USER app
COPY --from=build /out/service /usr/local/bin/service
ENTRYPOINT ["/usr/local/bin/service"]
