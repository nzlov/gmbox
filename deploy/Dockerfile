FROM node:25-alpine AS web-builder
WORKDIR /app
COPY package.json tsconfig.json ./
COPY web ./web
RUN npm install
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gmbox ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=go-builder /out/gmbox /app/gmbox
COPY config.yaml /app/config.yaml
EXPOSE 8080
CMD ["/app/gmbox"]
