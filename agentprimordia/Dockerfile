# ============================================================
# Stage 1: Go builder
# ============================================================
FROM golang:1.22-alpine AS go-builder

WORKDIR /build

COPY go.mod go.sum ./
RUN sed -i 's/^go .*/go 1.22/' go.mod && go mod download

COPY . .
RUN sed -i 's/^go .*/go 1.22/' go.mod

ENV CGO_ENABLED=0
RUN go build -ldflags="-s -w" -o /bin/ap-basic      ./examples/go/basic/ \
 && go build -ldflags="-s -w" -o /bin/ap-with-tools  ./examples/go/with-tools/ \
 && go build -ldflags="-s -w" -o /bin/ap-multi-agent ./examples/go/multi-agent/

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

WORKDIR /opt/ap-demo

CMD ["bash", "/opt/ap-demo/run-go.sh"]
