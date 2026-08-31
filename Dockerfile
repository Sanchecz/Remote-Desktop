FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN npm install -g pnpm@11.16.0 && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

FROM golang:1.26-alpine AS server-build
WORKDIR /src/server
COPY server/ ./
RUN go mod download && go vet -mod=readonly ./... && go test -mod=readonly ./... && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/genesis-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/remoteit-mcp-linux-amd64 ./cmd/mcp && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/RemoteIt-MCP.exe ./cmd/mcp

FROM golang:1.26-alpine AS agent-build
WORKDIR /src/agent
ARG LIBJPEG_TURBO_VERSION=3.2.0
ARG LIBJPEG_TURBO_SHA256=6f30092cef9fb839779646608f4ee14ae3cbac989c47fa05e841b0841f09878e
RUN apk add --no-cache cmake curl mingw-w64-gcc nasm ninja && \
    curl -fsSL "https://github.com/libjpeg-turbo/libjpeg-turbo/releases/download/${LIBJPEG_TURBO_VERSION}/libjpeg-turbo-${LIBJPEG_TURBO_VERSION}.tar.gz" -o /tmp/libjpeg-turbo.tar.gz && \
    echo "${LIBJPEG_TURBO_SHA256}  /tmp/libjpeg-turbo.tar.gz" | sha256sum -c - && \
    mkdir -p /tmp/libjpeg-turbo-src && tar -xzf /tmp/libjpeg-turbo.tar.gz --strip-components=1 -C /tmp/libjpeg-turbo-src && \
    cmake -S /tmp/libjpeg-turbo-src -B /tmp/libjpeg-turbo-build -G Ninja \
      -DCMAKE_SYSTEM_NAME=Windows \
      -DCMAKE_SYSTEM_PROCESSOR=AMD64 \
      -DCMAKE_C_COMPILER=x86_64-w64-mingw32-gcc \
      -DCMAKE_RC_COMPILER=x86_64-w64-mingw32-windres \
      -DCMAKE_INSTALL_PREFIX=/opt/libjpeg-turbo-windows \
      -DENABLE_SHARED=FALSE -DENABLE_STATIC=TRUE -DWITH_TURBOJPEG=TRUE \
      -DWITH_SIMD=TRUE -DWITH_JAVA=FALSE -DWITH_TESTS=FALSE && \
    cmake --build /tmp/libjpeg-turbo-build --target install --parallel && \
    test -f /opt/libjpeg-turbo-windows/lib/libturbojpeg.a && \
    rm -rf /tmp/libjpeg-turbo.tar.gz /tmp/libjpeg-turbo-src /tmp/libjpeg-turbo-build && \
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0
COPY agent/ ./
RUN /go/bin/goversioninfo -64 -icon=assets/genesisit.ico -application-icon=assets/genesisit.ico -manifest=assets/genesisit.exe.manifest \
      -o=cmd/agent/rsrc_windows_amd64.syso -company=RemoteIt -description="RemoteIt Agent" \
	-file-version=1.0.32.0 -product-version=1.0.32.0 -product-name=RemoteIt \
      -internal-name=RemoteItAgent -original-name=RemoteIt-Agent-Setup.exe \
	  -ver-major=1 -ver-minor=0 -ver-patch=32 -ver-build=0 \
	-product-ver-major=1 -product-ver-minor=0 -product-ver-patch=32 -product-ver-build=0 assets/versioninfo.json && \
    /go/bin/goversioninfo -64 -icon=assets/genesisit.ico -application-icon=assets/genesisit.ico -manifest=assets/genesisit.exe.manifest \
      -o=cmd/console/rsrc_windows_amd64.syso -company=RemoteIt -description="RemoteIt Console" \
	-file-version=1.0.32.0 -product-version=1.0.32.0 -product-name=RemoteIt \
      -internal-name=RemoteItConsole -original-name=RemoteIt-Console.exe \
	  -ver-major=1 -ver-minor=0 -ver-patch=32 -ver-build=0 \
	-product-ver-major=1 -product-ver-minor=0 -product-ver-patch=32 -product-ver-build=0 assets/versioninfo.json && \
    go mod download && go vet -mod=readonly ./... && go test -mod=readonly ./... && mkdir -p /out && \
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
      CGO_CFLAGS="-I/opt/libjpeg-turbo-windows/include" \
      CGO_LDFLAGS="-L/opt/libjpeg-turbo-windows/lib -static -static-libgcc" \
      go build -mod=readonly -trimpath -ldflags="-s -w -H=windowsgui" -o /out/remoteit-agent-windows-amd64.exe ./cmd/agent && \
    ! x86_64-w64-mingw32-objdump -p /out/remoteit-agent-windows-amd64.exe | grep -qi 'turbojpeg.dll' && \
    cp /out/remoteit-agent-windows-amd64.exe /out/RemoteIt-Agent-Setup.exe && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w -H=windowsgui" -o /out/RemoteIt-Console.exe ./cmd/console && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/remoteit-agent-linux-amd64 ./cmd/agent && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/remoteit-agent-macos-amd64 ./cmd/agent && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/remoteit-agent-macos-arm64 ./cmd/agent

FROM eclipse-temurin:17-jdk-jammy AS android-build
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget unzip && rm -rf /var/lib/apt/lists/*
ENV ANDROID_SDK_ROOT=/opt/android-sdk
RUN mkdir -p ${ANDROID_SDK_ROOT}/cmdline-tools && \
    wget -q https://dl.google.com/android/repository/commandlinetools-linux-15859902_latest.zip -O /tmp/android-tools.zip && \
    echo "4e4c464f145a7512b57d088ac6c278c03c9eea610886b35a5e0804e74eedf583  /tmp/android-tools.zip" | sha256sum -c - && \
    unzip -q /tmp/android-tools.zip -d ${ANDROID_SDK_ROOT}/cmdline-tools && \
    mv ${ANDROID_SDK_ROOT}/cmdline-tools/cmdline-tools ${ANDROID_SDK_ROOT}/cmdline-tools/latest && \
    rm /tmp/android-tools.zip
ENV PATH=${PATH}:${ANDROID_SDK_ROOT}/cmdline-tools/latest/bin:${ANDROID_SDK_ROOT}/platform-tools
RUN yes | sdkmanager --licenses >/dev/null || true && \
    sdkmanager "platforms;android-36" "build-tools;36.0.0" "platform-tools"
RUN wget -q https://services.gradle.org/distributions/gradle-9.5.0-bin.zip -O /tmp/gradle.zip && \
    wget -q https://services.gradle.org/distributions/gradle-9.5.0-bin.zip.sha256 -O /tmp/gradle.sha256 && \
    echo "$(cat /tmp/gradle.sha256)  /tmp/gradle.zip" | sha256sum -c - && \
    mkdir -p /opt/gradle && unzip -q /tmp/gradle.zip -d /opt/gradle && rm /tmp/gradle.zip /tmp/gradle.sha256
WORKDIR /src/mobile/android
COPY mobile/android/ ./
RUN --mount=type=secret,id=android_keystore \
    --mount=type=secret,id=android_signing \
    set -a && . /run/secrets/android_signing && set +a && \
    export GENESIS_ANDROID_KEYSTORE=/run/secrets/android_keystore && \
    /opt/gradle/gradle-9.5.0/bin/gradle --no-daemon assembleRelease && \
    ${ANDROID_SDK_ROOT}/build-tools/36.0.0/apksigner verify --verbose --print-certs app/build/outputs/apk/release/app-release.apk && \
    ${ANDROID_SDK_ROOT}/build-tools/36.0.0/apksigner verify --verbose --print-certs agentapp/build/outputs/apk/release/agentapp-release.apk

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S genesis && adduser -S -G genesis genesis
WORKDIR /app
COPY --from=server-build /out/genesis-server /app/genesis-server
COPY --from=web-build /src/web/dist /app/web
COPY --from=agent-build /out /app/web/downloads
COPY --from=server-build /out/RemoteIt-MCP.exe /app/web/downloads/RemoteIt-MCP.exe
COPY --from=server-build /out/remoteit-mcp-linux-amd64 /app/web/downloads/remoteit-mcp-linux-amd64
COPY installer/unix/install-remoteit.sh /app/web/downloads/install-remoteit.sh
COPY --from=android-build /src/mobile/android/app/build/outputs/apk/release/app-release.apk /app/web/downloads/RemoteIt.apk
COPY --from=android-build /src/mobile/android/agentapp/build/outputs/apk/release/agentapp-release.apk /app/web/downloads/RemoteIt-Agent-Android.apk
RUN cd /app/web/downloads && \
    sed -i 's/\r$//' install-remoteit.sh && \
    sh -n install-remoteit.sh && \
    WIN_SHA="$(sha256sum remoteit-agent-windows-amd64.exe | cut -d ' ' -f1)" && WIN_SIZE="$(wc -c < remoteit-agent-windows-amd64.exe | tr -d ' ')" && \
    LINUX_SHA="$(sha256sum remoteit-agent-linux-amd64 | cut -d ' ' -f1)" && LINUX_SIZE="$(wc -c < remoteit-agent-linux-amd64 | tr -d ' ')" && \
    MAC_AMD_SHA="$(sha256sum remoteit-agent-macos-amd64 | cut -d ' ' -f1)" && MAC_AMD_SIZE="$(wc -c < remoteit-agent-macos-amd64 | tr -d ' ')" && \
    MAC_ARM_SHA="$(sha256sum remoteit-agent-macos-arm64 | cut -d ' ' -f1)" && MAC_ARM_SIZE="$(wc -c < remoteit-agent-macos-arm64 | tr -d ' ')" && \
	printf '{"version":"1.0.32","platforms":{"windows-amd64":{"path":"/downloads/remoteit-agent-windows-amd64.exe","sha256":"%s","size":%s},"linux-amd64":{"path":"/downloads/remoteit-agent-linux-amd64","sha256":"%s","size":%s},"darwin-amd64":{"path":"/downloads/remoteit-agent-macos-amd64","sha256":"%s","size":%s},"darwin-arm64":{"path":"/downloads/remoteit-agent-macos-arm64","sha256":"%s","size":%s}}}\n' "$WIN_SHA" "$WIN_SIZE" "$LINUX_SHA" "$LINUX_SIZE" "$MAC_AMD_SHA" "$MAC_AMD_SIZE" "$MAC_ARM_SHA" "$MAC_ARM_SIZE" > AGENT-RELEASE.json && \
	sha256sum RemoteIt-Console.exe RemoteIt-Agent-Setup.exe RemoteIt-MCP.exe remoteit-mcp-linux-amd64 remoteit-agent-windows-amd64.exe remoteit-agent-linux-amd64 remoteit-agent-macos-amd64 remoteit-agent-macos-arm64 RemoteIt.apk RemoteIt-Agent-Android.apk install-remoteit.sh APK-SIGNER.txt AGENT-RELEASE.json > SHA256SUMS.txt
USER genesis
EXPOSE 8080
ENTRYPOINT ["/app/genesis-server"]
