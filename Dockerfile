FROM debian:trixie-slim
RUN apt update && apt upgrade -y && apt install -y bash bat gum curl wget
RUN cd /bin/ && ln -s batcat bat || true
RUN useradd username
RUN mkdir -p /home/username && chown 1000:1000 /home/username
USER 1000:1000
COPY ./tmp/otools /home/username/src/otools
COPY ./bin/orchard-tui /bin/
WORKDIR /home/username
ENTRYPOINT ["/bin/orchard-tui"]
