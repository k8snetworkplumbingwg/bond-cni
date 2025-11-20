# This Dockerfile is used to build the image available on DockerHub
FROM docker.io/golang:1.24 AS build

WORKDIR /usr/src/bond-cni
COPY . .
RUN make build-bin

FROM docker.io/alpine:latest
LABEL org.opencontainers.image.source=https://github.com/k8snetworkplumbingwg/bond-cni
WORKDIR /
COPY --from=build /usr/src/bond-cni/bin .
COPY LICENSE .
COPY images/entrypoint.sh .

CMD ["/entrypoint.sh"]
