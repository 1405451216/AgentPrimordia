# ============================================================
# Stage 1: Go builder
# ============================================================
FROM golang:1.23-alpine AS go-builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -ldflags="-s -w" -o /bin/ap-basic      ./ecosystem/examples/basic/ \
 && go build -ldflags="-s -w" -o /bin/ap-with-tools  ./ecosystem/examples/with-tools/ \
 && go build -ldflags="-s -w" -o /bin/ap-multi-agent ./ecosystem/examples/multi-agent/

# ============================================================
# Stage 2: Node builder
# ============================================================
FROM node:20-alpine AS node-builder

WORKDIR /build

COPY sdk/typescript/package.json sdk/typescript/tsconfig.json ./
COPY sdk/typescript/src/ ./src/

RUN npm install
RUN npm run build

# ============================================================
# Stage 3: Runtime
# ============================================================
FROM node:20-alpine

RUN apk add --no-cache bash

COPY --from=go-builder /bin/ap-basic      /usr/local/bin/ap-basic
COPY --from=go-builder /bin/ap-with-tools /usr/local/bin/ap-with-tools
COPY --from=go-builder /bin/ap-multi-agent /usr/local/bin/ap-multi-agent

COPY --from=node-builder /build/dist        /opt/ap-sdk/dist
COPY --from=node-builder /build/src         /opt/ap-sdk/src
COPY --from=node-builder /build/package.json /opt/ap-sdk/package.json
COPY --from=node-builder /build/node_modules /opt/ap-sdk/node_modules

COPY demo/ /opt/ap-demo/
RUN chmod +x /opt/ap-demo/run-go.sh /opt/ap-demo/run-ts.sh

# Security hardening
RUN addgroup -S ap && adduser -S ap -G ap
USER ap

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD pgrep -f "ap-basic" > /dev/null || exit 1

WORKDIR /opt/ap-demo

CMD ["bash", "/opt/ap-demo/run-go.sh"]
