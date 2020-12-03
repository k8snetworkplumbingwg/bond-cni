FROM golang:alpine as builder

COPY . /usr/src/bond-cni

WORKDIR /usr/src/bond-cni
RUN apk add --no-cache --virtual build-dependencies build-base=~0.5 && \
    make clean && \
    make build

FROM alpine:3
COPY --from=builder /usr/src/bond-cni/bond /usr/bin/
WORKDIR /

LABEL io.k8s.display-name="BOND CNI"

COPY ./images/entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
