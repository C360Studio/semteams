# Production Dockerfile for SemStreams UI
# Multi-stage build: Build SvelteKit with Node.js, run as Node.js server

# Stage 1: Build SvelteKit application
FROM node:22-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies (including devDependencies for build)
RUN npm ci

# Copy source code
COPY . .

# Build SvelteKit application with adapter-node
RUN npm run build

# Stage 2: Production runtime
FROM node:22-alpine

WORKDIR /app

# Copy built application from builder
COPY --from=builder /app/build ./build
COPY --from=builder /app/package*.json ./

# Install production dependencies only
RUN npm ci --omit=dev

# Expose port (SvelteKit default)
EXPOSE 3000

# Environment variables
ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

# Health check - SvelteKit serves on PORT
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

# Run as non-root user for security
RUN addgroup -g 1001 -S nodejs && adduser -S sveltekit -u 1001
USER sveltekit

# Start SvelteKit server
CMD ["node", "build"]
