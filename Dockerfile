# syntax=docker/dockerfile:1
# gl:docs/dev/go-redir-svc.md#L10
FROM golang:1.25.5 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/go-redir-svc ./

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/go-redir-svc ./go-redir-svc
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["./go-redir-svc"]
