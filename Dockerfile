# syntax=docker/dockerfile:1

# ---- Étape de dev : live-reload avec air ----
FROM golang:1.26.5 AS dev
WORKDIR /app

# Installer air pour le live-reload
RUN go install github.com/air-verse/air@latest

# Copier les fichiers de modules en premier
COPY go.mod ./
RUN go mod download

# Copier les fichiers source Go et la config air
COPY *.go .air.toml ./

EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# ---- Étape de build (prod) ----
FROM golang:1.26.5 AS build-stage
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o tamiyo

# ---- Étape finale : image minimale (prod) ----
FROM gcr.io/distroless/base-debian11 AS build-release-stage
WORKDIR /app
COPY --from=build-stage /app/tamiyo .
EXPOSE 8080
ENTRYPOINT ["/app/tamiyo"]
