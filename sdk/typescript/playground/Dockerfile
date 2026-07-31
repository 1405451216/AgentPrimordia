# AgentPrimordia Playground — 多阶段构建
# 构建：docker build -t ap-playground .
# 运行：docker run -d -p 8080:8080 -e AP_LLM_API_KEY=sk-xxx ap-playground

# ===== Stage 1: 构建 =====
FROM node:20-alpine AS builder

WORKDIR /app

# 复制 SDK 根目录的包配置
COPY sdk/typescript/package.json sdk/typescript/package-lock.json* ./sdk/typescript/
WORKDIR /app/sdk/typescript
RUN npm ci --ignore-scripts

# 复制源码并构建
COPY sdk/typescript/ ./
RUN npm run build

# ===== Stage 2: 运行时 =====
FROM node:20-alpine AS runtime

LABEL maintainer="AgentPrimordia Team"
LABEL description="AgentPrimordia Playground - Interactive Multi-Agent Environment"

WORKDIR /app

# 安全：使用非 root 用户
RUN addgroup -S apgroup && adduser -S apuser -G apgroup

# 仅复制构建产物和生产依赖
COPY --from=builder /app/sdk/typescript/dist ./dist
COPY --from=builder /app/sdk/typescript/package.json ./
RUN npm install --omit=dev --ignore-scripts && npm cache clean --force

# 环境变量
ENV NODE_ENV=production \
    AP_PLAYGROUND_PORT=8080 \
    AP_LLM_PROVIDER=openai \
    AP_LLM_MODEL=gpt-4 \
    AP_MAX_AGENTS=50 \
    AP_LOG_LEVEL=info

EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:${AP_PLAYGROUND_PORT}/healthz || exit 1

USER apuser

# 启动 Playground 服务
CMD ["node", "dist/playground/index.js"]
