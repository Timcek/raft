FROM golang:1.24.3

WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/main

EXPOSE 5000
CMD ["/app/main"]