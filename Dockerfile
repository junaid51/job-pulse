# Two stages so the thing that ships is a binary and a text file, nothing else.
# Built inside the image rather than cross-compiled, because the host this runs
# on is ARM (Oracle's Ampere) and a laptop is not.
FROM golang:1.26-alpine AS build
WORKDIR /src
# Dependencies first: they change far less often than the code, so this layer
# survives most rebuilds.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/jobpulse ./cmd/jobpulse

FROM alpine:3.21
# Certificates, because every job board is HTTPS, and zoneinfo because quiet
# hours are worked out in the reader's own timezone.
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/jobpulse /app/jobpulse
# The board list is read at boot, so it has to be in the image. Migrations are
# embedded in the binary already.
COPY companies.txt /app/companies.txt
ENV PORT=8080
EXPOSE 8080
# No shell wrapper: the process is PID 1 and gets the signals directly, so a
# restart is a restart rather than a ten-second wait for a kill.
ENTRYPOINT ["/app/jobpulse"]
