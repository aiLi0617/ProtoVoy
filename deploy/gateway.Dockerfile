FROM rust:1.88-bookworm AS build
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake g++ make pkg-config clang libclang-dev perl nasm \
    && rm -rf /var/lib/apt/lists/*
COPY . .
RUN cargo build --locked --release -p protovoy-protocol-gateway

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/target/release/protovoy-protocol-gateway /usr/local/bin/protovoy-protocol-gateway
EXPOSE 39219
ENTRYPOINT ["protovoy-protocol-gateway"]
