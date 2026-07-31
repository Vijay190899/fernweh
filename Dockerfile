# One Dockerfile for every service in the monorepo; the target binary is
# selected with a build arg so images stay identical in shape.
FROM golang:1.26-alpine AS build
ARG CMD=gateway
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/app ./cmd/${CMD}

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /bin/app /bin/app
ENTRYPOINT ["/bin/app"]
