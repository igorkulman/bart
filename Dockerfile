FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o bart .

FROM alpine:3.21
COPY --from=build /app/bart /bart
VOLUME ["/assets", "/config.yml"]
EXPOSE 8080
ENTRYPOINT ["/bart", "-config", "/config.yml", "-assets", "/assets"]
