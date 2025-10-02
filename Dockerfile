# Stage 1: Build the Go application
FROM golang:1.22-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to leverage Docker's layer caching.
# This step will only be re-run if these files change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application as a static binary. CGO_ENABLED=0 is crucial for
# creating a static binary that can run in a minimal container like alpine.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gokv .

# Stage 2: Create the final, minimal image
FROM alpine:latest

COPY --from=builder /app/gokv /usr/local/bin/gokv

EXPOSE 8080
CMD ["gokv", "--db", "/data/gokv.db"]