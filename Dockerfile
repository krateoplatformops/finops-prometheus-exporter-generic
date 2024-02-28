FROM scratch
WORKDIR /prometheus-exporter-generic
COPY ./bin ./bin
WORKDIR /temp
WORKDIR /prometheus-exporter-generic/bin
ENTRYPOINT ["./prometheus-exporter-generic"]