# ---- Build Stage ----
# Use the official Go image to build the application
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /src

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application for a Linux environment, statically linked
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/service-faas ./cmd/service-faas

# ---- Final Stage ----
# Use a minimal base image
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the built binary from the builder stage
COPY --from=builder /app/service-faas .

# (Optional) Copy any other necessary files, like configs or templates
# COPY config.yaml .

# Expose the port the application listens on
EXPOSE 8080

# The command to run the application
CMD ["./service-faas"]