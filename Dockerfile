# Build layer
FROM golang:1.18 AS builder
WORKDIR /go/src/github.com/aireuropa/resec
COPY . .
ARG RESEC_VERSION
ENV RESEC_VERSION ${RESEC_VERSION:-local-dev}
RUN echo $RESEC_VERSION
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'main.Version=${RESEC_VERSION}'" -a -installsuffix cgo -o build/resec  .

# Run layer
FROM alpine:3.18.3
RUN apk --no-cache add ca-certificates && \
export exe=`exec 2>/dev/null; readlink "/proc/$$/exe"| rev | cut  -f 1 -d '/' | rev` && \
case "$exe" in \
    'busybox') \
        echo "Busybox: $exe"; \
        getent group  1000 || addgroup -g 1000 -S app; \
        getent passwd 1000 || adduser -S -G app -u 1000 app; \
        ;; \
    *) \
        echo "Not Busybox: $exe"; \
        getent group  1000 || addgroup --gid 1000 --system  app; \
        getent passwd 1000 || adduser --system --no-create-home --ingroup app  --uid 1000 app; \
        ;; \
esac && \
mkdir /app/ && \
chown 1000:1000 /app
USER 1000
WORKDIR /app 
COPY --from=builder /go/src/github.com/aireuropa/resec/build/resec .
CMD ["./resec"]
